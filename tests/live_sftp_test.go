package tests

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/config"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLiveSFTP(t *testing.T) {
	if os.Getenv("SEUSC_LIVE_SFTP") != "1" {
		t.Skip("requires explicit SEUSC_LIVE_SFTP=1 and authenticated agent")
	}
	p, e := platform.DefaultPaths()
	if e != nil {
		t.Fatal(e)
	}
	cfg, e := config.Load(p.ConfigDir)
	if e != nil {
		t.Fatal(e)
	}
	keyBytes, e := os.ReadFile(filepath.Join(p.SSHDir, "seusc_ed25519"))
	if e != nil {
		t.Fatal(e)
	}
	hostBytes, e := os.ReadFile(filepath.Join(p.DataDir, "ssh_host_ed25519_key"))
	if e != nil {
		t.Fatal(e)
	}
	key, e := ssh.ParsePrivateKey(keyBytes)
	if e != nil {
		t.Fatal(e)
	}
	host, e := ssh.ParsePrivateKey(hostBytes)
	if e != nil {
		t.Fatal(e)
	}
	conn, e := ssh.Dial("tcp", net.JoinHostPort(cfg.SSH.Listen, strconv.Itoa(cfg.SSH.Port)), &ssh.ClientConfig{User: "seusc", Auth: []ssh.AuthMethod{ssh.PublicKeys(key)}, HostKeyCallback: ssh.FixedHostKey(host.PublicKey()), Timeout: 10 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	client, e := sftp.NewClient(conn)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	id := make([]byte, 8)
	rand.Read(id)
	dir := "seu-bridge-test-" + hex.EncodeToString(id)
	if e = client.Mkdir(dir); e != nil {
		t.Fatal(e)
	}
	names := []string{"large.bin", "empty.txt", "中文 空格.json"}
	defer func() {
		for _, name := range names {
			_ = client.Remove(dir + "/" + name)
		}
		_ = client.RemoveDirectory(dir)
	}()
	large := make([]byte, 11<<20+71)
	rand.Read(large)
	data := [][]byte{large, {}, []byte(`{"bridge":"中文","success":false,"code":51}`)}
	for i, name := range names {
		f, e := client.Create(dir + "/" + name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(data[i]); e != nil {
			t.Fatal(e)
		}
		if e = f.Truncate(int64(len(data[i]))); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		r, e := client.Open(dir + "/" + name)
		if e != nil {
			t.Fatal(e)
		}
		got, e := io.ReadAll(r)
		r.Close()
		if e != nil || sha256.Sum256(got) != sha256.Sum256(data[i]) {
			t.Fatal("hash mismatch", name, e)
		}
		t.Log("SHA-256 verified:", name)
	}
	entries, e := client.ReadDir(dir)
	if e != nil || len(entries) != 3 {
		t.Fatalf("directory list count=%d error=%v", len(entries), e)
	}
	short := []byte("overwritten")
	f, e := client.OpenFile(dir+"/large.bin", os.O_WRONLY|os.O_CREATE)
	if e != nil {
		t.Fatal(e)
	}
	f.Write(short)
	if e = f.Truncate(int64(len(short))); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	r, e := client.Open(dir + "/large.bin")
	if e != nil {
		t.Fatal(e)
	}
	got, e := io.ReadAll(r)
	r.Close()
	if e != nil || !bytes.Equal(got, short) {
		t.Fatal("overwrite failed")
	}
	if e = client.RemoveDirectory(dir); e == nil {
		t.Fatal("nonempty directory removal must fail")
	}
	t.Log("directory listing, overwrite and nonempty directory protection verified")
}
