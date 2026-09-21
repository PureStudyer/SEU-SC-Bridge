package tests

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/auth"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/sshserver"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := ssh.NewSignerFromKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSSHWebShell(t *testing.T) {
	resized := make(chan [2]int, 16)
	closed := make(chan struct{}, 16)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Authorization") != "Bearer opaque" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("NodeId") != "9" {
			t.Error("wrong configured node")
		}
		c, e := websocket.Accept(w, r, nil)
		if e != nil {
			return
		}
		defer c.CloseNow()
		defer func() { closed <- struct{}{} }()
		c.Write(r.Context(), websocket.MessageBinary, []byte("welcome 中文\r\n"))
		for {
			kind, b, e := c.Read(r.Context())
			if e != nil {
				return
			}
			if kind == websocket.MessageText {
				var v struct {
					Type       string
					Rows, Cols int
				}
				if json.Unmarshal(b, &v) == nil && v.Type == "resize" {
					resized <- [2]int{v.Rows, v.Cols}
				}
				continue
			}
			if strings.HasPrefix(string(b), "stty -echo;") {
				re := regexp.MustCompile(`'([a-f0-9]{32})__'`)
				m := re.FindStringSubmatch(string(b))
				if len(m) != 2 {
					t.Error("missing nonce")
					return
				}
				id := m[1]
				output := "echoed " + string(b) + "\r\n\x1e__SEUSC_BEGIN_" + id + "__\x1fmock-host\r\n\x1e__SEUSC_DONE_" + id + "__:7\x1f"
				for _, part := range []byte(output) {
					if c.Write(r.Context(), websocket.MessageBinary, []byte{part}) != nil {
						return
					}
				}
				continue
			}
			if string(b) == "disconnect" {
				return
			}
			if c.Write(r.Context(), websocket.MessageBinary, b) != nil {
				return
			}
		}
	}))
	defer mock.Close()
	host, key := signer(t), signer(t)
	tokens := auth.New(func(context.Context) (auth.Credentials, error) { return auth.Credentials{Token: "opaque"}, nil })
	server := sshserver.New(host, key.PublicKey(), func(ctx context.Context, rows, cols int) (webshell.Session, error) {
		return tokens.Connect(ctx, &webshell.Client{BaseURL: mock.URL}, 9, rows, cols)
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go server.Serve(ctx, l)
	defer func() { cancel(); l.Close(); server.DisconnectAll(); server.Wait() }()
	cfg := &ssh.ClientConfig{User: "seusc", Auth: []ssh.AuthMethod{ssh.PublicKeys(key)}, HostKeyCallback: ssh.FixedHostKey(host.PublicKey()), Timeout: 3 * time.Second}
	dial := func() (*ssh.Client, error) { return ssh.Dial("tcp", l.Addr().String(), cfg) }
	t.Run("wrong key rejected", func(t *testing.T) {
		bad := *cfg
		bad.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer(t))}
		if c, e := ssh.Dial("tcp", l.Addr().String(), &bad); e == nil {
			c.Close()
			t.Fatal("accepted unauthorized key")
		}
	})
	t.Run("interactive raw bytes and resize", func(t *testing.T) {
		c, e := dial()
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		s, e := c.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		in, _ := s.StdinPipe()
		out, _ := s.StdoutPipe()
		if e = s.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{}); e != nil {
			t.Fatal(e)
		}
		if e = s.Shell(); e != nil {
			t.Fatal(e)
		}
		banner := make([]byte, len("welcome 中文\r\n"))
		if _, e = io.ReadFull(out, banner); e != nil {
			t.Fatal(e)
		}
		want := []byte("\x03\x1a\x1c\t\x1b[A中文\x00\xff")
		if _, e = in.Write(want); e != nil {
			t.Fatal(e)
		}
		got := make([]byte, len(want))
		if _, e = io.ReadFull(out, got); e != nil || !bytes.Equal(got, want) {
			t.Fatal("raw bytes changed", e)
		}
		if e = s.WindowChange(40, 120); e != nil {
			t.Fatal(e)
		}
		select {
		case size := <-resized:
			if size != [2]int{40, 120} {
				t.Fatal(size)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("resize not forwarded")
		}
		if e = s.Signal(ssh.SIGINT); e != nil {
			t.Fatal(e)
		}
		one := make([]byte, 1)
		if _, e = io.ReadFull(out, one); e != nil || one[0] != 3 {
			t.Fatal("signal failed", e)
		}
	})
	t.Run("exec output and exit status", func(t *testing.T) {
		c, e := dial()
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		s, _ := c.NewSession()
		defer s.Close()
		out, e := s.CombinedOutput("hostname")
		status, ok := e.(*ssh.ExitError)
		if !ok || status.ExitStatus() != 7 || string(out) != "mock-host\r\n" {
			t.Fatalf("out=%q err=%v", out, e)
		}
	})
	t.Run("concurrent sessions", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c, e := dial()
				if e != nil {
					t.Error(e)
					return
				}
				defer c.Close()
				s, e := c.NewSession()
				if e != nil {
					t.Error(e)
					return
				}
				defer s.Close()
				in, _ := s.StdinPipe()
				out, _ := s.StdoutPipe()
				if e = s.Shell(); e != nil {
					t.Error(e)
					return
				}
				banner := make([]byte, len("welcome 中文\r\n"))
				io.ReadFull(out, banner)
				want := fmt.Sprintf("client-%d", i)
				io.WriteString(in, want)
				b := make([]byte, len(want))
				if _, e = io.ReadFull(out, b); e != nil || string(b) != want {
					t.Errorf("session isolation: %q %v", b, e)
				}
			}(i)
		}
		wg.Wait()
	})
	t.Run("upstream disconnect closes SSH", func(t *testing.T) {
		c, e := dial()
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		s, _ := c.NewSession()
		defer s.Close()
		in, _ := s.StdinPipe()
		out, _ := s.StdoutPipe()
		s.Shell()
		b := make([]byte, len("welcome 中文\r\n"))
		io.ReadFull(out, b)
		io.WriteString(in, "disconnect")
		done := make(chan error, 1)
		go func() { _, e := io.ReadAll(out); done <- e }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("SSH session leaked")
		}
	})
	t.Run("unsupported SFTP rejected", func(t *testing.T) {
		c, e := dial()
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		s, _ := c.NewSession()
		defer s.Close()
		if s.RequestSubsystem("sftp") == nil {
			t.Fatal("SFTP should be rejected")
		}
	})
	// Every established terminal must have closed after its SSH channel closed.
	for i := 0; i < 8; i++ {
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("upstream connection leaked")
		}
	}
}
