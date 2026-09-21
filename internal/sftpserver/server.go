// Package sftpserver exposes the SEU web file service through standard SFTP v3.
package sftpserver

import (
	"context"
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/finder"
	"github.com/pkg/sftp"
	"io"
	"os"
	"path"
	"strings"
	"sync"
)

type Backend interface {
	Stat(context.Context, string) (finder.Entry, error)
	List(context.Context, string) ([]finder.Entry, error)
	Mkdir(context.Context, string) error
	Rmdir(context.Context, string) error
	Rename(context.Context, string, string) error
	Remove(context.Context, string, bool) error
	Download(context.Context, string, io.Writer) error
	Upload(context.Context, string, io.ReaderAt, int64) error
}
type Handler struct {
	mu            sync.Mutex
	uploads       map[string]*writer
	Context       context.Context
	FS            Backend
	Home, TempDir string
}

func (h *Handler) Resolve(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", os.ErrInvalid
	}
	if p == "" || p == "." || p == "~" || p == "/~" {
		return h.Home, nil
	}
	if strings.HasPrefix(p, "~/") {
		p = path.Join(h.Home, p[2:])
	}
	if strings.HasPrefix(p, "/~"+"/") {
		p = path.Join(h.Home, p[3:])
	}
	if !path.IsAbs(p) {
		p = path.Join(h.Home, p)
	}
	return path.Clean(p), nil
}
func (h *Handler) RealPath(p string) (string, error) { return h.Resolve(p) }
func (h *Handler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	p, e := h.Resolve(r.Filepath)
	if e != nil {
		return nil, e
	}
	switch r.Method {
	case "List":
		entries, e := h.FS.List(r.Context(), p)
		if e != nil {
			return nil, e
		}
		list := make([]os.FileInfo, len(entries))
		for i := range entries {
			list[i] = entries[i]
		}
		return infoList(list), nil
	case "Stat", "Lstat":
		entry, e := h.FS.Stat(r.Context(), p)
		if e != nil {
			return nil, e
		}
		return infoList{entry}, nil
	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}
func (h *Handler) Lstat(r *sftp.Request) (sftp.ListerAt, error) { return h.Filelist(r) }
func (h *Handler) Readlink(p string) (string, error) {
	p, e := h.Resolve(p)
	if e != nil {
		return "", e
	}
	entry, e := h.FS.Stat(h.Context, p)
	if e != nil {
		return "", e
	}
	if !entry.Symlink {
		return "", os.ErrInvalid
	}
	return entry.LinkPath, nil
}

type infoList []os.FileInfo

func (l infoList) ListAt(out []os.FileInfo, offset int64) (int, error) {
	if offset < 0 {
		return 0, os.ErrInvalid
	}
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(out, l[offset:])
	if n < len(out) {
		return n, io.EOF
	}
	return n, nil
}
func (h *Handler) temp() (*os.File, error) {
	if e := os.MkdirAll(h.TempDir, 0700); e != nil {
		return nil, e
	}
	return os.CreateTemp(h.TempDir, "transfer-*")
}

type reader struct{ *os.File }

func (r *reader) Close() error { p := r.Name(); e := r.File.Close(); _ = os.Remove(p); return e }
func (h *Handler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	p, e := h.Resolve(r.Filepath)
	if e != nil {
		return nil, e
	}
	stat, e := h.FS.Stat(r.Context(), p)
	if e != nil {
		return nil, e
	}
	if stat.IsDir() {
		return nil, os.ErrInvalid
	}
	f, e := h.temp()
	if e != nil {
		return nil, e
	}
	result := &reader{f}
	if e = h.FS.Download(r.Context(), p, f); e != nil {
		result.Close()
		return nil, e
	}
	size, e := f.Stat()
	if e != nil || size.Size() != stat.Size() {
		result.Close()
		return nil, errors.New("download size mismatch; remote file may have changed")
	}
	return result, nil
}

type writer struct {
	mu             sync.Mutex
	file           *os.File
	handler        *Handler
	ctx            context.Context
	dest           string
	failed, closed bool
}

func (w *writer) WriteAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || offset < 0 {
		return 0, os.ErrInvalid
	}
	n, e := w.file.WriteAt(p, offset)
	if e != nil {
		w.failed = true
	}
	return n, e
}
func (w *writer) TransferError(error) { w.mu.Lock(); w.failed = true; w.mu.Unlock() }
func (w *writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	defer func() { w.handler.mu.Lock(); delete(w.handler.uploads, w.dest); w.handler.mu.Unlock() }()
	defer func() { name := w.file.Name(); w.file.Close(); os.Remove(name) }()
	if w.failed || w.ctx.Err() != nil {
		return errors.New("upload cancelled")
	}
	stat, e := w.file.Stat()
	if e != nil {
		return e
	}
	return w.handler.FS.Upload(w.ctx, w.dest, w.file, stat.Size())
}
func (h *Handler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.uploads == nil {
		h.uploads = map[string]*writer{}
	}
	p, e := h.Resolve(r.Filepath)
	if e != nil {
		return nil, e
	}
	if h.uploads[p] != nil {
		return nil, errors.New("file already open for upload")
	}
	flags := r.Pflags()
	if flags.Append {
		return nil, sftp.ErrSSHFxOpUnsupported
	}
	entry, e := h.FS.Stat(r.Context(), p)
	if e == nil {
		if entry.IsDir() {
			return nil, os.ErrInvalid
		}
		if flags.Excl {
			return nil, os.ErrExist
		}

	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	} else if !flags.Creat {
		return nil, os.ErrNotExist
	}
	parent, e := h.FS.Stat(r.Context(), path.Dir(p))
	if e != nil {
		return nil, e
	}
	if !parent.IsDir() {
		return nil, os.ErrInvalid
	}
	f, e := h.temp()
	if e != nil {
		return nil, e
	}
	if e == nil && !flags.Trunc && entry.Filename != "" {
		if err := h.FS.Download(r.Context(), p, f); err != nil {
			f.Close()
			os.Remove(f.Name())
			return nil, err
		}
	}
	w := &writer{file: f, handler: h, ctx: h.Context, dest: p}
	h.uploads[p] = w
	return w, nil
}
func (h *Handler) Filecmd(r *sftp.Request) error {
	p, e := h.Resolve(r.Filepath)
	if e != nil {
		return e
	}
	switch r.Method {
	case "Setstat":
		flags := r.AttrFlags()
		if !flags.Size || flags.Permissions || flags.Acmodtime || flags.UidGid {
			return sftp.ErrSSHFxOpUnsupported
		}
		h.mu.Lock()
		w := h.uploads[p]
		h.mu.Unlock()
		if w == nil {
			return sftp.ErrSSHFxOpUnsupported
		}
		size := r.Attributes().Size
		if size > 1<<63-1 {
			return os.ErrInvalid
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.closed {
			return os.ErrClosed
		}
		return w.file.Truncate(int64(size))
	case "Mkdir":
		return h.FS.Mkdir(r.Context(), p)
	case "Rename", "PosixRename":
		dest, e := h.Resolve(r.Target)
		if e != nil {
			return e
		}
		return h.FS.Rename(r.Context(), p, dest)
	case "Remove":
		entry, e := h.FS.Stat(r.Context(), p)
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return os.ErrInvalid
		}
		return h.FS.Remove(r.Context(), p, false)
	case "Rmdir":
		return h.FS.Rmdir(r.Context(), p)
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}
func (h *Handler) PosixRename(r *sftp.Request) error { return h.Filecmd(r) }
func (h *Handler) Serve(ch io.ReadWriteCloser) error {
	server := sftp.NewRequestServer(channelStream{ch}, sftp.Handlers{FileGet: h, FilePut: h, FileCmd: h, FileList: h}, sftp.WithStartDirectory(h.Home))
	defer server.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-h.Context.Done():
			ch.Close()
			server.Close()
		case <-done:
		}
	}()
	e := server.Serve()
	if errors.Is(e, io.EOF) {
		return nil
	}
	return e
}

func (w *writer) ReadAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	return w.file.ReadAt(p, offset)
}
func (h *Handler) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	w, e := h.Filewrite(r)
	if e != nil {
		return nil, e
	}
	return w.(*writer), nil
}

type channelStream struct{ io.ReadWriteCloser }

func (channelStream) Close() error { return nil }
