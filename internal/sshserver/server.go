package sshserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/bridge"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"golang.org/x/crypto/ssh"
)

type Connector func(context.Context, int, int) (webshell.Session, error)
type Server struct {
	SFTP    func(context.Context, ssh.Channel) error
	Config  *ssh.ServerConfig
	Connect Connector
	Log     *slog.Logger
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	wg      sync.WaitGroup
}

func New(host ssh.Signer, client ssh.PublicKey, connect Connector, log *slog.Logger) *Server {
	cfg := &ssh.ServerConfig{MaxAuthTries: 3, ServerVersion: "SSH-2.0-SEUSC", PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() != "seusc" || !bytes.Equal(k.Marshal(), client.Marshal()) {
			return nil, errors.New("unauthorized local key")
		}
		return nil, nil
	}}
	cfg.AddHostKey(host)
	return &Server{Config: cfg, Connect: connect, Log: log, conns: make(map[net.Conn]struct{})}
}
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() { <-ctx.Done(); l.Close(); s.DisconnectAll() }()
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		s.mu.Lock()
		if len(s.conns) >= 64 {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go s.handle(ctx, conn)
	}
}
func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.Close()
	}
}
func (s *Server) Wait() { s.wg.Wait() }
func (s *Server) handle(parent context.Context, raw net.Conn) {
	defer s.wg.Done()
	defer func() { raw.Close(); s.mu.Lock(); delete(s.conns, raw); s.mu.Unlock() }()
	raw.SetDeadline(time.Now().Add(15 * time.Second))
	conn, chans, reqs, err := ssh.NewServerConn(raw, s.Config)
	if err != nil {
		return
	}
	raw.SetDeadline(time.Time{})
	defer conn.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() { conn.Wait(); cancel() }()
	go ssh.DiscardRequests(reqs)
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 16)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			nc.Reject(ssh.ResourceShortage, "too many sessions")
			continue
		}
		ch, requests, e := nc.Accept()
		if e != nil {
			<-slots
			continue
		}
		wg.Add(1)
		go func() { defer wg.Done(); defer func() { <-slots }(); s.session(ctx, ch, requests) }()
	}
}

type PTY struct {
	Term                      string
	Cols, Rows, Width, Height uint32
	Modes                     string
}
type Window struct{ Cols, Rows, Width, Height uint32 }

func ParsePTY(p []byte) (PTY, error) {
	var v PTY
	e := ssh.Unmarshal(p, &v)
	if e == nil {
		e = dimensions(v.Rows, v.Cols)
	}
	return v, e
}
func ParseWindow(p []byte) (Window, error) {
	var v Window
	e := ssh.Unmarshal(p, &v)
	if e == nil {
		e = dimensions(v.Rows, v.Cols)
	}
	return v, e
}
func dimensions(rows, cols uint32) error {
	if rows == 0 || cols == 0 || rows > 65535 || cols > 65535 {
		return errors.New("invalid terminal dimensions")
	}
	return nil
}
func exit(ch ssh.Channel, code uint32) {
	_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
}
func (s *Server) session(parent context.Context, ch ssh.Channel, requests <-chan *ssh.Request) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer ch.Close()
	rows, cols := 24, 80
	started := false
	var ws webshell.Session
	var wsMu sync.Mutex
	// Closing a channel while its authentication/handshake is pending must cancel that operation too.
	incoming := make(chan *ssh.Request, 16)
	go func() {
		defer close(incoming)
		for {
			select {
			case r, ok := <-requests:
				if !ok {
					cancel()
					return
				}
				select {
				case incoming <- r:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		wsMu.Lock()
		if ws != nil {
			ws.Close()
		}
		wsMu.Unlock()
	}()
	done := make(chan uint32, 1)
	for {
		select {
		case <-ctx.Done():
			return
		case code := <-done:
			exit(ch, code)
			return
		case req, ok := <-incoming:
			if !ok {
				return
			}
			accepted := false
			switch req.Type {
			case "pty-req":
				if !started {
					if v, e := ParsePTY(req.Payload); e == nil {
						wsMu.Lock()
						rows, cols = int(v.Rows), int(v.Cols)
						wsMu.Unlock()
						accepted = true
					}
				}
			case "window-change":
				if v, e := ParseWindow(req.Payload); e == nil {
					wsMu.Lock()
					rows, cols = int(v.Rows), int(v.Cols)
					wsMu.Unlock()
					accepted = true
					wsMu.Lock()
					if ws != nil {
						accepted = ws.Resize(ctx, rows, cols) == nil
					}
					wsMu.Unlock()
				}
			case "shell", "exec":
				if started {
					break
				}
				command := ""
				if req.Type == "exec" {
					var v struct{ Command string }
					if ssh.Unmarshal(req.Payload, &v) != nil {
						break
					}
					command = v.Command
				} else if len(req.Payload) != 0 {
					break
				}
				started = true
				accepted = true
				initialRows, initialCols := rows, cols
				ready := make(chan webshell.Session, 1)
				isExec := req.Type == "exec"
				go func() {
					session, err := s.Connect(ctx, initialRows, initialCols)
					if err != nil {
						io.WriteString(ch.Stderr(), "SEUSC: 无法连接 WebShell，请运行 seusc status / seusc login。\r\n")
						done <- 255
						return
					}
					wsMu.Lock()
					ws = session
					if rows != initialRows || cols != initialCols {
						_ = session.Resize(ctx, rows, cols)
					}
					wsMu.Unlock()
					ready <- session
					defer session.Close()
					s.Log.Info("ssh session opened")
					defer s.Log.Info("ssh session closed")
					// Receive-side requests continue while the handshake is in flight.
					if isExec {
						code, e := bridge.Exec(ctx, ch, session, command)
						if e != nil {
							s.Log.Warn("exec did not complete")
							io.WriteString(ch.Stderr(), "SEUSC: command did not complete\r\n")
						}
						done <- code
					} else {
						_, _ = io.Copy(ch, session)
						done <- 0
					}
				}()
				go func() {
					select {
					case <-ctx.Done():
						return
					case session := <-ready:
						if !isExec {
							_, _ = io.Copy(session, ch)
						}
					}
				}()
			case "subsystem":
				var sub struct{ Name string }
				if started || s.SFTP == nil || ssh.Unmarshal(req.Payload, &sub) != nil || sub.Name != "sftp" {
					break
				}
				started = true
				accepted = true
				go func() {
					code := uint32(0)
					if s.SFTP(ctx, ch) != nil {
						code = 255
					}
					done <- code
				}()
			case "signal":
				var v struct{ Signal string }
				if ssh.Unmarshal(req.Payload, &v) == nil {
					signals := map[string]byte{"INT": 3, "TSTP": 26, "QUIT": 28}
					if b, ok := signals[v.Signal]; ok {
						wsMu.Lock()
						if ws != nil {
							_, e := ws.Write([]byte{b})
							accepted = e == nil
						}
						wsMu.Unlock()
					}
				}
			}
			if req.WantReply {
				req.Reply(accepted, nil)
			}
		}
	}
}
func Listen(address string, port int) (net.Listener, int, error) {
	ip := net.ParseIP(address)
	if ip == nil || !ip.IsLoopback() {
		return nil, 0, errors.New("SSH must listen on a loopback address")
	}
	for p := port; p < port+20 && p <= 65535; p++ {
		l, e := net.Listen("tcp", net.JoinHostPort(address, fmt.Sprint(p)))
		if e == nil {
			return l, p, nil
		}
	}
	return nil, 0, errors.New("SSH_PORT_IN_USE: no free local SSH port")
}
