//go:build !windows

package ipc

import (
	"context"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"net"
	"os"
)

// Caller holds the agent lock, so only a stale socket can exist here.
func Listen(p platform.Paths) (net.Listener, error) {
	if e := os.Remove(p.Socket); e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	l, e := net.Listen("unix", p.Socket)
	if e != nil {
		return nil, e
	}
	if e = os.Chmod(p.Socket, 0600); e != nil {
		l.Close()
		return nil, e
	}
	return l, nil
}
func Dial(ctx context.Context, p platform.Paths) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", p.Socket)
}
