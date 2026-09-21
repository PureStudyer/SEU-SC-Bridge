package sftpserver

import (
	"bytes"
	"context"
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/finder"
	"github.com/pkg/sftp"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type localFS struct {
	root    string
	uploads int
}

func (l *localFS) p(p string) string { return filepath.Join(l.root, filepath.FromSlash(p)) }
func entry(f os.FileInfo) finder.Entry {
	return finder.Entry{Filename: f.Name(), Bytes: f.Size(), Directory: f.IsDir(), Permissions: "0644", Modified: f.ModTime()}
}
func (l *localFS) Stat(c context.Context, p string) (finder.Entry, error) {
	f, e := os.Stat(l.p(p))
	if e != nil {
		return finder.Entry{}, e
	}
	return entry(f), nil
}
func (l *localFS) List(c context.Context, p string) ([]finder.Entry, error) {
	files, e := os.ReadDir(l.p(p))
	if e != nil {
		return nil, e
	}
	var list []finder.Entry
	for _, f := range files {
		i, e := f.Info()
		if e != nil {
			return nil, e
		}
		list = append(list, entry(i))
	}
	return list, nil
}
func (l *localFS) Mkdir(c context.Context, p string) error          { return os.Mkdir(l.p(p), 0700) }
func (l *localFS) Rename(c context.Context, a, b string) error      { return os.Rename(l.p(a), l.p(b)) }
func (l *localFS) Remove(c context.Context, p string, d bool) error { return os.Remove(l.p(p)) }
func (l *localFS) Download(c context.Context, p string, w io.Writer) error {
	f, e := os.Open(l.p(p))
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = io.Copy(w, f)
	return e
}
func (l *localFS) Upload(c context.Context, p string, r io.ReaderAt, size int64) error {
	l.uploads++
	f, e := os.Create(l.p(p))
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = io.Copy(f, io.NewSectionReader(r, 0, size))
	return e
}
func TestProtocolRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &localFS{root: t.TempDir()}
	os.MkdirAll(backend.p("/home/test"), 0700)
	h := &Handler{Context: ctx, FS: backend, Home: "/home/test", TempDir: t.TempDir()}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan error, 1)
	go func() { done <- h.Serve(a) }()
	client, e := sftp.NewClientPipe(b, b)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	cwd, e := client.Getwd()
	if e != nil || cwd != "/home/test" {
		t.Fatal(cwd, e)
	}
	for _, name := range []string{"中文 空格.bin", "empty.txt"} {
		f, e := client.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		data := []byte{}
		if name != "empty.txt" {
			data = bytes.Repeat([]byte{0, 255, 3, 26, 10, 13}, 15000)
		}
		if _, e = f.Write(data); e != nil {
			t.Fatal(e)
		}
		if e = f.Truncate(int64(len(data))); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		r, e := client.Open(name)
		if e != nil {
			t.Fatal(e)
		}
		got, e := io.ReadAll(r)
		r.Close()
		if e != nil || !bytes.Equal(got, data) {
			t.Fatal("binary mismatch", e)
		}
	}
	files, e := client.ReadDir(".")
	if e != nil || len(files) != 2 {
		t.Fatal("listing", e, len(files))
	}
	if e = client.Mkdir("sub"); e != nil {
		t.Fatal(e)
	}
	if e = client.Rename("empty.txt", "sub/new.txt"); e != nil {
		t.Fatal(e)
	}
	if e = client.RemoveDirectory("sub"); e == nil {
		t.Fatal("deleted nonempty directory")
	}
	if e = client.Remove("sub/new.txt"); e != nil {
		t.Fatal(e)
	}
	if e = client.RemoveDirectory("sub"); e != nil {
		t.Fatal(e)
	}
	if _, e = client.OpenFile("中文 空格.bin", os.O_WRONLY|os.O_APPEND); e == nil {
		t.Fatal("append unsupported but accepted")
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session leaked")
	}
	tmp, e := os.ReadDir(h.TempDir)
	if e != nil || len(tmp) != 0 {
		t.Fatal("temporary transfer files leaked")
	}
}
func TestAbortedUploadDoesNotPublish(t *testing.T) {
	backend := &localFS{root: t.TempDir()}
	f, e := os.CreateTemp(t.TempDir(), "transfer")
	if e != nil {
		t.Fatal(e)
	}
	w := &writer{file: f, handler: &Handler{FS: backend}, ctx: context.Background(), dest: "/aborted"}
	w.WriteAt([]byte("partial"), 0)
	w.TransferError(errors.New("disconnected"))
	if e = w.Close(); e == nil {
		t.Fatal("abort accepted")
	}
	if backend.uploads != 0 {
		t.Fatal("partial upload published")
	}
	if _, e = os.Stat(f.Name()); !os.IsNotExist(e) {
		t.Fatal("spool not removed")
	}
}

func (l *localFS) Rmdir(c context.Context, p string) error { return os.Remove(l.p(p)) }
