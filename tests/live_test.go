package tests

import (
	"bytes"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	sync.Mutex
	b bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) { b.Lock(); defer b.Unlock(); return b.b.Write(p) }
func (b *lockedBuffer) String() string              { b.Lock(); defer b.Unlock(); return b.b.String() }
func TestLiveInteractive(t *testing.T) {
	if os.Getenv("SEUSC_LIVE_TEST") != "1" {
		t.Skip("set SEUSC_LIVE_TEST=1 and SEUSC_HOME to an authenticated test agent")
	}
	home := os.Getenv("SEUSC_HOME")
	if home == "" {
		t.Fatal("explicit SEUSC_HOME required")
	}
	b, e := os.ReadFile(filepath.Join(home, ".ssh", "seusc_ed25519"))
	if e != nil {
		t.Fatal(e)
	}
	key, e := ssh.ParsePrivateKey(b)
	if e != nil {
		t.Fatal(e)
	}
	hostBytes, e := os.ReadFile(filepath.Join(home, "ssh_host_ed25519_key"))
	if e != nil {
		t.Fatal(e)
	}
	host, e := ssh.ParsePrivateKey(hostBytes)
	if e != nil {
		t.Fatal(e)
	}
	c, e := ssh.Dial("tcp", "127.0.0.1:24822", &ssh.ClientConfig{User: "seusc", Auth: []ssh.AuthMethod{ssh.PublicKeys(key)}, HostKeyCallback: ssh.FixedHostKey(host.PublicKey()), Timeout: 10 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	s, e := c.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	out := &lockedBuffer{}
	s.Stdout = out
	s.Stderr = out
	in, _ := s.StdinPipe()
	if e = s.RequestPty("xterm-256color", 24, 120, ssh.TerminalModes{}); e != nil {
		t.Fatal(e)
	}
	if e = s.Shell(); e != nil {
		t.Fatal(e)
	}

	wait := func(marker string) {
		t.Helper()
		deadline := time.NewTimer(12 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			if strings.Contains(out.String(), marker) {
				return
			}
			select {
			case <-deadline.C:
				t.Fatalf("terminal did not produce %q", marker)
			case <-tick.C:
			}
		}
	}
	io.WriteString(in, "printf '__SEUSC_%s__\\n' LIVE_READY\r")
	wait("__SEUSC_LIVE_READY__")
	io.WriteString(in, "printf '__SEUSC_%s__\\n' 中文输出\r")
	wait("__SEUSC_中文输出__")
	if e = s.WindowChange(40, 120); e != nil {
		t.Fatal(e)
	}
	io.WriteString(in, "stty size; printf '__SEUSC_%s__\\n' RESIZE_DONE\r")
	wait("__SEUSC_RESIZE_DONE__")
	if !strings.Contains(out.String(), "40 120") {
		t.Fatal("remote PTY did not resize")
	}
	io.WriteString(in, "sleep 30\r")
	time.Sleep(300 * time.Millisecond)
	in.Write([]byte{3})
	io.WriteString(in, "printf '__SEUSC_%s__\\n' INTERRUPTED\r")
	wait("__SEUSC_INTERRUPTED__")
	io.WriteString(in, "printf '__SEUSC_%s__\\n' ARROW_EDIT\r")
	wait("__SEUSC_ARROW_EDIT__")
	in.Write([]byte("\x1b[A\r"))
	time.Sleep(200 * time.Millisecond)
	io.WriteString(in, "exit\r")
	t.Log("interactive input, UTF-8, PTY resize and Ctrl+C passed")

}
