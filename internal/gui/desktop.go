//go:build desktop

package gui

import (
	"context"

	"github.com/PureStudyer/SEU-SC-Bridge/frontend"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/agent"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/ipc"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"github.com/energye/systray"
	"github.com/gofrs/flock"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"
)

type App struct {
	paths platform.Paths
	ctx   context.Context
	ready chan struct{}
	once  sync.Once
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx; a.once.Do(func() { close(a.ready) }) }
func (a *App) Status() (agent.Status, error) {
	ctx, c := context.WithTimeout(a.ctx, 3*time.Second)
	defer c()
	var st agent.Status
	e := ipc.Call(ctx, a.paths, ipc.Request{Op: "status"}, &st)
	return st, e
}
func (a *App) Start() error { return agent.Start(a.ctx, a.paths) }
func (a *App) Login(username, password string, remember, autostart bool) (agent.Status, error) {
	var st agent.Status
	if e := a.Start(); e != nil {
		return st, e
	}
	e := ipc.Call(a.ctx, a.paths, ipc.Request{Op: "login", Username: username, Password: password, Remember: remember}, &st)
	if e == nil {
		e = ipc.Call(a.ctx, a.paths, ipc.Request{Op: "settings", AutoStart: &autostart}, nil)
	}
	return st, e
}
func (a *App) Logout() error { return ipc.Call(a.ctx, a.paths, ipc.Request{Op: "logout"}, nil) }
func (a *App) Restart() error {
	ctx, c := context.WithTimeout(a.ctx, 15*time.Second)
	defer c()
	_ = agent.Stop(ctx, a.paths)
	return a.Start()
}
func (a *App) Stop() error {
	ctx, c := context.WithTimeout(a.ctx, 15*time.Second)
	defer c()
	return agent.Stop(ctx, a.paths)
}
func (a *App) SetNode(name string, id int) error {
	return ipc.Call(a.ctx, a.paths, ipc.Request{Op: "set-default-node", Node: name, NodeID: id}, nil)
}
func (a *App) SetAutoStart(enabled bool) error {
	return ipc.Call(a.ctx, a.paths, ipc.Request{Op: "settings", AutoStart: &enabled}, nil)
}
func (a *App) Logs() (string, error) {
	var s string
	e := ipc.Call(a.ctx, a.paths, ipc.Request{Op: "get-logs"}, &s)
	return s, e
}
func (a *App) CopyCommand() error {
	st, e := a.Status()
	if e != nil {
		return e
	}
	return runtime.ClipboardSetText(a.ctx, "ssh "+st.Alias)
}
func Run(p platform.Paths) error     { return run(p, false) }
func RunTray(p platform.Paths) error { return run(p, true) }
func run(p platform.Paths, hidden bool) error {
	platform.PrepareGUI()
	lock := flock.New(filepath.Join(p.DataDir, "gui.lock"))
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	gp := p
	gp.DataDir = p.DataDir + "-gui"
	gp.Socket = filepath.Join(p.DataDir, "gui.sock")
	if !ok {
		if hidden {
			return nil
		}
		ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		return ipc.Call(ctx, gp, ipc.Request{Op: "open"}, nil)
	}
	defer lock.Unlock()
	a := &App{paths: p, ready: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l, e := ipc.Listen(gp)
	if e != nil {
		return e
	}
	defer l.Close()
	go ipc.Serve(ctx, l, func(c context.Context, r ipc.Request) ipc.Response {
		select {
		case <-a.ready:
		case <-c.Done():
			return ipc.Reply(nil, c.Err())
		}
		if r.Op == "quit" {
			runtime.Quit(a.ctx)
		} else {
			runtime.WindowShow(a.ctx)
			runtime.WindowUnminimise(a.ctx)
		}
		return ipc.Reply(nil, nil)
	})
	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(trayIcon())
		systray.SetTitle("SEU SC Bridge")
		systray.SetTooltip("SEU SC Bridge · 本地 SSH")
		open := systray.AddMenuItem("打开 SEU SC Bridge", "打开主窗口")
		open.Click(func() { <-a.ready; runtime.WindowShow(a.ctx); runtime.WindowUnminimise(a.ctx) })
		copy := systray.AddMenuItem("复制 SSH 命令", "复制 ssh seusc")
		copy.Click(func() { <-a.ready; _ = a.CopyCommand() })
		restart := systray.AddMenuItem("重启代理", "重新启动后台进程")
		restart.Click(func() { go func() { <-a.ready; _ = a.Restart() }() })
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 SEU SC Bridge", "停止后台代理并退出")
		quit.Click(func() { go func() { <-a.ready; _ = a.Stop(); runtime.Quit(a.ctx) }() })
	}, func() {})
	defer end()
	return wails.Run(&options.App{Title: "SEU SC Bridge", Width: 1020, Height: 740, MinWidth: 800, MinHeight: 620, StartHidden: hidden, HideWindowOnClose: true, BackgroundColour: &options.RGBA{R: 14, G: 20, B: 30, A: 255}, AssetServer: &assetserver.Options{Assets: frontend.Assets}, OnStartup: func(c context.Context) { a.startup(c); start() }, OnShutdown: func(context.Context) { cancel() }, Bind: []interface{}{a}})
}
func trayIcon() []byte {
	if goruntime.GOOS == "windows" {
		return frontend.IconICO
	}
	return frontend.TrayPNG
}
