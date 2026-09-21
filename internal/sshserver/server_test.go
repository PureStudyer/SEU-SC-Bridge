package sshserver

import (
	"golang.org/x/crypto/ssh"
	"testing"
)

func TestRequests(t *testing.T) {
	v, e := ParsePTY(ssh.Marshal(PTY{Term: "xterm-256color", Rows: 24, Cols: 80, Modes: string([]byte{0})}))
	if e != nil || v.Rows != 24 || v.Cols != 80 {
		t.Fatal(v, e)
	}
	if _, e = ParsePTY([]byte{0, 1}); e == nil {
		t.Fatal("accepted malformed request")
	}
	if _, e = ParseWindow(ssh.Marshal(Window{Rows: 0, Cols: 80})); e == nil {
		t.Fatal("accepted zero dimensions")
	}
	if _, _, e := Listen("0.0.0.0", 24822); e == nil {
		t.Fatal("public listener permitted")
	}
}
