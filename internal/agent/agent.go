package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/auth"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/bridge"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/browser"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/config"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/credential"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/finder"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/ipc"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/logging"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/sftpserver"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/sshserver"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"github.com/gofrs/flock"
	"golang.org/x/crypto/ssh"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var Version = "1.0.0"

type Status struct {
	Version   string         `json:"version"`
	State     string         `json:"state"`
	Account   string         `json:"account"`
	Node      string         `json:"node"`
	Nodes     map[string]int `json:"nodes"`
	Address   string         `json:"address"`
	Alias     string         `json:"alias"`
	Remember  bool           `json:"remember"`
	AutoStart bool           `json:"autostart"`
	ErrorCode string         `json:"error_code,omitempty"`
	PID       int            `json:"pid"`
}
type Agent struct {
	commandMu             sync.Mutex
	mu                    sync.Mutex
	authOp                sync.Mutex
	paths                 platform.Paths
	cfg                   config.Config
	state, code, password string
	visible               bool
	authCancel            context.CancelFunc
	tokens                *auth.Manager
	store                 credential.Store
	log                   *slog.Logger
	server                *sshserver.Server
	cancel                context.CancelFunc
	ctx                   context.Context
}

func Run(parent context.Context, p platform.Paths) error {
	lock := flock.New(p.Lock)
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		return apperr.New("ALREADY_RUNNING", "SEUSC 后台进程已运行")
	}
	defer lock.Unlock()
	cfg, e := config.Load(p.ConfigDir)
	if e != nil {
		return e
	}
	log, rotate := logging.New(p.LogDir, cfg.Logging.Level)
	defer rotate.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	a := &Agent{paths: p, cfg: cfg, state: "STARTING", store: credential.System{}, log: log, cancel: cancel, ctx: ctx}
	a.tokens = auth.New(a.acquire)
	host, key, e := config.EnsureSSH(p, cfg)
	if e != nil {
		return e
	}
	listener, port, e := sshserver.Listen(cfg.SSH.Listen, cfg.SSH.Port)
	if e != nil {
		return e
	}
	defer listener.Close()
	a.cfg.SSH.Port = port
	if e = config.WriteSSH(p, a.cfg, host.PublicKey()); e != nil {
		return e
	}
	if e = config.Save(p.ConfigDir, a.cfg); e != nil {
		return e
	}
	a.server = sshserver.New(host, key, a.connect, log)
	a.server.SFTP = a.serveSFTP
	local, e := ipc.Listen(p)
	if e != nil {
		return e
	}
	defer local.Close()
	go func() {
		if e := a.server.Serve(ctx, listener); e != nil && ctx.Err() == nil {
			log.Error("SSH listener failed")
			cancel()
		}
	}()
	go func() {
		if e := ipc.Serve(ctx, local, a.handle); e != nil && ctx.Err() == nil {
			log.Error("IPC listener failed")
			cancel()
		}
	}()
	platform.StartTray()
	a.setState("AUTH_REQUIRED", "")
	log.Info("agent started", "address", listener.Addr().String())
	if cfg.Username != "" {
		go func() { _, _ = a.tokens.Get(ctx) }()
	}
	<-ctx.Done()
	a.setState("STOPPED", "")
	a.cancelAuth()
	a.authOp.Lock()
	a.authOp.Unlock()
	a.server.DisconnectAll()
	a.server.Wait()
	log.Info("agent stopped")
	return nil
}
func (a *Agent) setState(state, code string) {
	a.mu.Lock()
	a.state = state
	a.code = code
	a.mu.Unlock()
}
func (a *Agent) status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	nodes := map[string]int{}
	for n, id := range a.cfg.Nodes {
		nodes[n] = id
	}
	return Status{Version: Version, State: a.state, Account: a.cfg.Username, Node: a.cfg.SEU.DefaultNode, Nodes: nodes, Address: fmt.Sprintf("%s:%d", a.cfg.SSH.Listen, a.cfg.SSH.Port), Alias: a.cfg.SSH.Alias, Remember: a.cfg.Remember, AutoStart: a.cfg.Agent.AutoStart, ErrorCode: a.code, PID: os.Getpid()}
}
func (a *Agent) acquire(ctx context.Context) (auth.Credentials, error) {
	a.authOp.Lock()
	defer a.authOp.Unlock()
	a.mu.Lock()
	cfg := a.cfg
	password := a.password
	visible := a.visible
	a.visible = false
	ac, cancel := context.WithCancel(ctx)
	a.authCancel = cancel
	a.mu.Unlock()
	defer cancel()
	defer func() { a.mu.Lock(); a.authCancel = nil; a.mu.Unlock() }()
	if cfg.Username == "" {
		return auth.Credentials{}, apperr.New("AUTH_REQUIRED", "请先登录 SEU 账号")
	}
	if password == "" && cfg.Remember {
		var e error
		password, e = a.store.Get(cfg.Username)
		if e != nil {
			a.setState("ERROR", apperr.Code(e))
			return auth.Credentials{}, e
		}
	}
	a.setState("AUTHENTICATING", "")
	creds, e := (browser.Login{Profile: a.paths.Profile, Config: cfg, Username: cfg.Username, Password: password, Visible: visible, Progress: func(state string) { a.log.Debug("browser login progress", "state", state) }}).Acquire(ac)
	if e != nil {
		a.setState("AUTH_REQUIRED", apperr.Code(e))
		a.log.Warn("authentication required", "code", apperr.Code(e))
		return auth.Credentials{}, e
	}
	a.setState("READY", "")
	a.log.Info("authentication acquired")
	return creds, nil
}
func (a *Agent) connect(ctx context.Context, rows, cols int) (webshell.Session, error) {
	a.mu.Lock()
	node := a.cfg.Nodes[a.cfg.SEU.DefaultNode]
	base := a.cfg.SEU.BaseURL
	inputMode := a.cfg.SEU.InputMode
	a.mu.Unlock()
	ws, e := a.tokens.Connect(ctx, &webshell.Client{BaseURL: base, InputMode: inputMode}, node, rows, cols)
	if e != nil {
		if e == webshell.ErrUnauthorized {
			a.setState("AUTH_REQUIRED", "TOKEN_EXPIRED")
		}
		return nil, e
	}
	a.log.Info("webshell connected", "node_id", node)
	return ws, nil
}
func (a *Agent) cancelAuth() {
	a.mu.Lock()
	if a.authCancel != nil {
		a.authCancel()
	}
	a.mu.Unlock()
	a.tokens.Invalidate()
}
func (a *Agent) handle(ctx context.Context, r ipc.Request) ipc.Response {
	if r.Op == "login" || r.Op == "logout" || r.Op == "restart" {
		a.cancelAuth()
	}
	switch r.Op {
	case "login", "logout", "restart", "settings", "set-default-node":
		a.commandMu.Lock()
		defer a.commandMu.Unlock()
	}
	switch r.Op {
	case "status":
		return ipc.Reply(a.status(), nil)
	case "stop":
		go func() { time.Sleep(100 * time.Millisecond); a.cancel() }()
		return ipc.Reply(nil, nil)
	case "restart":
		a.cancelAuth()
		a.server.DisconnectAll()
		go func() { _, _ = a.tokens.Get(a.ctx) }()
		return ipc.Reply(a.status(), nil)
	case "login":
		defer func() { a.mu.Lock(); a.password = ""; a.mu.Unlock() }()
		a.cancelAuth()
		a.authOp.Lock()
		a.mu.Lock()
		old := a.cfg.Username
		a.mu.Unlock()
		if r.Username == "" {
			a.authOp.Unlock()
			return ipc.Reply(nil, apperr.New("AUTH_REQUIRED", "请输入账号"))
		}
		if old != "" && old != r.Username {
			if e := a.store.Delete(old); e != nil {
				a.authOp.Unlock()
				return ipc.Reply(nil, e)
			}
			a.server.DisconnectAll()
			if e := os.RemoveAll(a.paths.Profile); e != nil {
				a.authOp.Unlock()
				return ipc.Reply(nil, e)
			}
		}
		a.mu.Lock()
		a.cfg.Username = r.Username
		a.cfg.Remember = r.Remember
		a.password = r.Password
		a.visible = r.Visible
		cfg := a.cfg
		a.mu.Unlock()
		a.authOp.Unlock()
		if e := config.Save(a.paths.ConfigDir, cfg); e != nil {
			return ipc.Reply(nil, e)
		}
		if !r.Remember {
			if e := a.store.Delete(r.Username); e != nil {
				return ipc.Reply(nil, e)
			}
		}
		_, e := a.tokens.Get(ctx)
		if e == nil && r.Remember && r.Password != "" {
			e = a.store.Set(r.Username, r.Password)
		}
		a.mu.Lock()
		a.password = ""
		a.mu.Unlock()
		return ipc.Reply(a.status(), e)
	case "logout":
		a.cancelAuth()
		a.authOp.Lock()
		defer a.authOp.Unlock()
		a.server.DisconnectAll()
		a.mu.Lock()
		user := a.cfg.Username
		a.mu.Unlock()
		if e := a.store.Delete(user); e != nil {
			return ipc.Reply(nil, e)
		}
		if e := os.RemoveAll(a.paths.Profile); e != nil {
			return ipc.Reply(nil, e)
		}
		a.mu.Lock()
		a.password = ""
		a.cfg.Username = ""
		a.cfg.Remember = false
		cfg := a.cfg
		a.mu.Unlock()
		a.setState("AUTH_REQUIRED", "")
		return ipc.Reply(a.status(), config.Save(a.paths.ConfigDir, cfg))
	case "set-default-node":
		a.mu.Lock()
		defer a.mu.Unlock()
		copyBytes, _ := json.Marshal(a.cfg)
		var next config.Config
		_ = json.Unmarshal(copyBytes, &next)
		if r.NodeID > 0 {
			next.Nodes[r.Node] = r.NodeID
		}
		next.SEU.DefaultNode = r.Node
		if e := config.Save(a.paths.ConfigDir, next); e != nil {
			return ipc.Reply(nil, e)
		}
		a.cfg = next
		return ipc.Reply(nil, nil)
	case "settings":
		if r.AutoStart == nil {
			return ipc.Reply(nil, apperr.New("INTERNAL_ERROR", "缺少设置"))
		}
		exe, e := os.Executable()
		if e == nil {
			e = platform.AutoStart(exe, *r.AutoStart)
		}
		if e != nil {
			return ipc.Reply(nil, e)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		a.cfg.Agent.AutoStart = *r.AutoStart
		return ipc.Reply(nil, config.Save(a.paths.ConfigDir, a.cfg))
	case "get-logs":
		f, e := os.Open(filepath.Join(a.paths.LogDir, "seusc.log"))
		if os.IsNotExist(e) {
			return ipc.Reply("", nil)
		}
		if e != nil {
			return ipc.Reply(nil, e)
		}
		defer f.Close()
		if s, e := f.Stat(); e == nil && s.Size() > 64<<10 {
			f.Seek(-(64 << 10), io.SeekEnd)
		}
		b, e := io.ReadAll(io.LimitReader(f, 64<<10))
		return ipc.Reply(string(b), e)
	case "check-webshell":
		s, e := a.connect(ctx, 24, 80)
		if e == nil {
			s.Close()
		}
		return ipc.Reply(nil, e)
	case "check-site":
		client := http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://sc.seu.edu.cn", nil)
		res, e := client.Do(req)
		if e != nil {
			return ipc.Reply(nil, apperr.New("SEU_UNREACHABLE", "SEU 网站不可达"))
		}
		res.Body.Close()
		return ipc.Reply(res.StatusCode, nil)
	default:
		return ipc.Reply(nil, apperr.New("INTERNAL_ERROR", "未知操作"))
	}
}

func (a *Agent) serveSFTP(ctx context.Context, ch ssh.Channel) error {
	a.mu.Lock()
	account := a.cfg.Username
	base := a.cfg.SEU.BaseURL
	a.mu.Unlock()
	ws, e := a.connect(ctx, 24, 120)
	if e != nil {
		return e
	}
	var out bytes.Buffer
	code, e := bridge.Exec(ctx, &out, ws, "printf '%s' \"$HOME\"")
	ws.Close()
	if e != nil || code != 0 {
		return errors.New("could not determine remote home directory")
	}
	home := strings.TrimSpace(out.String())
	if !strings.HasPrefix(home, "/") {
		return errors.New("invalid remote home directory")
	}
	a.log.Info("sftp session opened")
	defer a.log.Info("sftp session closed")
	h := &sftpserver.Handler{Context: ctx, FS: &finder.Client{BaseURL: base, Account: account, Tokens: a.tokens, RemoveEmptyDir: func(c context.Context, p string) error {
		session, e := a.connect(c, 24, 120)
		if e != nil {
			return e
		}
		defer session.Close()
		var output bytes.Buffer
		code, e := bridge.Exec(c, &output, session, "rmdir -- '"+strings.ReplaceAll(p, "'", `'\''`)+"'")
		if e != nil {
			return e
		}
		if code != 0 {
			return errors.New("directory is not empty, missing or inaccessible")
		}
		return nil
	}}, Home: home, TempDir: filepath.Join(a.paths.DataDir, "transfers")}
	return h.Serve(ch)
}
