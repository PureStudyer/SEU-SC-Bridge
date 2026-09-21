package agent

import (
	"context"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/ipc"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"os"
	"os/exec"
	"time"
)

func Start(ctx context.Context, p platform.Paths) error {
	probe, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	var st Status
	e := ipc.Call(probe, p, ipc.Request{Op: "status"}, &st)
	cancel()
	if e == nil {
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	cmd := exec.Command(exe, "agent")
	platform.Detached(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	_ = cmd.Process.Release()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return apperr.New("INTERNAL_ERROR", "后台启动失败，请运行 seusc agent 查看错误")
		case <-tick.C:
			probe, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
			e := ipc.Call(probe, p, ipc.Request{Op: "status"}, &st)
			cancel()
			if e == nil {
				return nil
			}
		}
	}
}
func Stop(ctx context.Context, p platform.Paths) error {
	if e := ipc.Call(ctx, p, ipc.Request{Op: "stop"}, nil); e != nil {
		return e
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			probe, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			e := ipc.Call(probe, p, ipc.Request{Op: "status"}, nil)
			cancel()
			if e != nil {
				return nil
			}
		}
	}
}
