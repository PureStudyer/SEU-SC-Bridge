//go:build windows

package ipc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/Microsoft/go-winio"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"golang.org/x/sys/windows"
	"net"
)

func pipeName(p platform.Paths) string {
	sum := sha256.Sum256([]byte(p.DataDir))
	return fmt.Sprintf(`\\.\pipe\seusc-agent-%x`, sum[:8])
}
func Listen(p platform.Paths) (net.Listener, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return nil, e
	}
	return winio.ListenPipe(pipeName(p), &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + u.User.Sid.String() + ")", InputBufferSize: 65536, OutputBufferSize: 65536})
}
func Dial(ctx context.Context, p platform.Paths) (net.Conn, error) {
	return winio.DialPipeContext(ctx, pipeName(p))
}
