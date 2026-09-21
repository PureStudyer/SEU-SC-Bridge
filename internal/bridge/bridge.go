package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"io"
	"strconv"
	"strings"
	"time"
)

// Interactive preserves every terminal byte, including control characters and ANSI escapes.
func Interactive(ctx context.Context, ch io.ReadWriteCloser, ws webshell.Session) error {
	done := make(chan error, 2)
	go func() { _, e := io.Copy(ws, ch); done <- e }()
	go func() { _, e := io.Copy(ch, ws); done <- e }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	ws.Close()
	ch.Close()
	return err
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Exec uses control-delimited random markers. Marker bytes never appear literally in the echoed command.
func Exec(ctx context.Context, ch io.Writer, ws webshell.Session, command string) (uint32, error) {
	if strings.ContainsRune(command, 0) {
		return 1, errors.New("NUL in command")
	}
	nonce := make([]byte, 16)
	if _, e := rand.Read(nonce); e != nil {
		return 1, e
	}
	id := hex.EncodeToString(nonce)
	begin := "__SEUSC_BEGIN_" + id + "__"
	end := "__SEUSC_DONE_" + id + "__"
	script := fmt.Sprintf(`stty -echo; printf '\036%%s%%s\037' '__SEUSC_BEGIN_' '%s__'; ( eval %s ) </dev/null; __seusc_rc=$?; printf '\036%%s%%s:%%d\037' '__SEUSC_DONE_' '%s__' "$__seusc_rc"; exit`+"\r", id, quote(command), id)
	// Linux canonical PTYs commonly cap a line at 4096 bytes. Fail explicitly rather than truncate.
	if len(script) > 3500 {
		return 1, errors.New("exec command is too long for WebShell PTY (3500-byte wrapper limit)")
	}
	if _, e := ws.Write([]byte(script)); e != nil {
		return 1, e
	}
	timer := time.AfterFunc(30*time.Second, func() { _ = ws.Close() })
	defer timer.Stop()
	return readExec(ch, ws, []byte("\x1e"+begin+"\x1f"), []byte("\x1e"+end+":"), func() { timer.Stop() })
}

// ReadExec bounds buffering and handles markers split across arbitrary frames. Ordinary output is streamed unchanged.
func ReadExec(dst io.Writer, src io.Reader, begin, end []byte) (uint32, error) {
	return readExec(dst, src, begin, end, func() {})
}
func readExec(dst io.Writer, src io.Reader, begin, end []byte, onStart func()) (uint32, error) {
	started := false
	buf := make([]byte, 0, 32768)
	chunk := make([]byte, 16384)
	for {
		n, e := src.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if !started {
			if i := bytes.Index(buf, begin); i >= 0 {
				buf = buf[i+len(begin):]
				started = true
				onStart()
			} else if len(buf) > len(begin) {
				buf = buf[len(buf)-len(begin):]
			}
		}
		if started {
			if i := bytes.Index(buf, end); i >= 0 {
				if _, err := dst.Write(buf[:i]); err != nil {
					return 1, err
				}
				buf = buf[i:]
				if j := bytes.IndexByte(buf[len(end):], 0x1f); j >= 0 {
					v, err := strconv.ParseUint(string(buf[len(end):len(end)+j]), 10, 8)
					if err != nil {
						return 1, errors.New("invalid exec exit status")
					}
					return uint32(v), nil
				}
				if len(buf) > len(end)+4 {
					return 1, errors.New("invalid exec completion marker")
				}
			} else if len(buf) > len(end) {
				count := len(buf) - len(end)
				if _, err := dst.Write(buf[:count]); err != nil {
					return 1, err
				}
				buf = buf[count:]
			}
		}
		if e != nil {
			if started && len(buf) > 0 {
				_, _ = dst.Write(buf)
			}
			return 255, errors.New("WebShell closed before command completion")
		}
	}
}
