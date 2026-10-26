package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/ipc"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
)

func Start(ctx context.Context, p platform.Paths) error {
	probe, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
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
	for attempt := 0; attempt < 2; attempt++ {
		childErr, exited := startAgentProcess(ctx, p, exe)
		if childErr == nil {
			return nil
		}
		if exited && strings.Contains(childErr.Error(), "ALREADY_RUNNING") {
			// Let an agent that is still initializing publish its IPC endpoint
			// before treating the lock owner as stale.
			if err := waitContext(ctx, time.Second); err != nil {
				return err
			}
			if agentResponds(ctx, p) {
				return nil
			}
			if recovered, recoveryErr := platform.RecoverStaleAgent(p, exe); recoveryErr != nil {
				return apperr.New("INTERNAL_ERROR", fmt.Sprintf("无法恢复无响应的后台进程：%v", recoveryErr))
			} else if recovered && attempt == 0 {
				continue
			}
			return apperr.New("INTERNAL_ERROR", "已有后台进程占用单实例锁，但控制通道无响应；请运行 seusc stop 后重试")
		}
		if !exited && attempt == 0 && ctx.Err() == nil {
			continue
		}
		return apperr.New("INTERNAL_ERROR", fmt.Sprintf("后台代理无法启动：%v", childErr))
	}
	return apperr.New("INTERNAL_ERROR", "后台代理重启后仍未响应，请运行 seusc doctor 检查")
}

func startAgentProcess(ctx context.Context, p platform.Paths, exe string) (error, bool) {
	cmd := exec.Command(exe, "agent")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	platform.Detached(cmd)
	if err := cmd.Start(); err != nil {
		return err, true
	}
	childDone := make(chan error, 1)
	go func() { childDone <- cmd.Wait() }()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-childDone
			return ctx.Err(), false
		case childErr := <-childDone:
			message := strings.TrimSpace(output.String())
			if message != "" {
				return fmt.Errorf("%s", message), true
			}
			if childErr == nil {
				childErr = fmt.Errorf("agent process exited before IPC became available")
			}
			return childErr, true
		case <-timeout.C:
			_ = cmd.Process.Kill()
			<-childDone
			return fmt.Errorf("agent process did not expose IPC within 10 seconds"), false
		case <-tick.C:
			if agentResponds(ctx, p) {
				return nil, false
			}
		}
	}
}

func agentResponds(ctx context.Context, p platform.Paths) bool {
	probe, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	var st Status
	return ipc.Call(probe, p, ipc.Request{Op: "status"}, &st) == nil
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func Stop(ctx context.Context, p platform.Paths) error {
	if e := ipc.Call(ctx, p, ipc.Request{Op: "stop"}, nil); e != nil {
		if exe, err := os.Executable(); err == nil {
			if recovered, recoveryErr := platform.RecoverStaleAgent(p, exe); recovered || recoveryErr != nil {
				return recoveryErr
			}
		}
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
