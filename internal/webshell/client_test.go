package webshell

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestURL(t *testing.T) {
	raw, e := URL("https://sc.seu.edu.cn", ConnectOptions{Token: "opaque +&/= token", NodeID: 7, Rows: 40, Cols: 120})
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(raw)
	if u.Scheme != "wss" || u.Path != "/finder/v2/webshell" || u.Query().Get("Authorization") != "Bearer opaque +&/= token" || u.Query().Get("NodeId") != "7" {
		t.Fatal("incorrect WebShell URL")
	}
}
func TestResize(t *testing.T) {
	b, e := ResizeMessage(40, 120)
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil || v["type"] != "resize" || v["rows"] != float64(40) || v["cols"] != float64(120) {
		t.Fatal(string(b))
	}
	if _, e = ResizeMessage(0, 120); e == nil {
		t.Fatal("accepted invalid dimensions")
	}
}
func TestAuthenticationFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer s.Close()
	_, e := (&Client{BaseURL: s.URL}).Connect(context.Background(), ConnectOptions{Token: "DO_NOT_LEAK", NodeID: 6, Rows: 24, Cols: 80})
	if e != ErrUnauthorized {
		t.Fatalf("wrong error: %v", e)
	}
}
func TestFramesAndCookies(t *testing.T) {
	done := make(chan error, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "http://"+r.Host || r.Header.Get("Cookie") != "session=value" {
			t.Error("missing origin/cookie")
		}
		c, e := websocket.Accept(w, r, nil)
		if e != nil {
			done <- e
			return
		}
		defer c.CloseNow()
		kind, b, e := c.Read(r.Context())
		if e != nil {
			done <- e
			return
		}
		if kind != websocket.MessageBinary {
			t.Error("input is not binary")
		}
		done <- c.Write(r.Context(), websocket.MessageBinary, b)
	}))
	defer s.Close()
	ws, e := (&Client{BaseURL: s.URL}).Connect(context.Background(), ConnectOptions{Token: "opaque", Cookies: []*http.Cookie{{Name: "session", Value: "value"}}, NodeID: 6, Rows: 24, Cols: 80})
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	want := []byte{0, 3, 26, 27, 0xff}
	if _, e = ws.Write(want); e != nil {
		t.Fatal(e)
	}
	got := make([]byte, 32)
	n, e := ws.Read(got)
	if e != nil || string(got[:n]) != string(want) {
		t.Fatal("bytes modified", e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}

func TestJSONInputSplitUTF8(t *testing.T) {
	received := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := websocket.Accept(w, r, nil)
		if e != nil {
			return
		}
		defer c.CloseNow()
		for {
			kind, b, e := c.Read(r.Context())
			if e != nil {
				return
			}
			if kind != websocket.MessageText {
				t.Error("JSON mode must send text frames")
			}
			var msg struct {
				Type string
				Data string
			}
			if e = json.Unmarshal(b, &msg); e != nil || msg.Type != "input" {
				t.Error("bad input envelope")
			}
			received <- msg.Data
		}
	}))
	defer srv.Close()
	ws, e := (&Client{BaseURL: srv.URL, InputMode: "json"}).Connect(context.Background(), ConnectOptions{Token: "opaque", NodeID: 6, Rows: 24, Cols: 80})
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	raw := []byte("中文\x03\x1a")
	for _, b := range raw {
		if _, e = ws.Write([]byte{b}); e != nil {
			t.Fatal(e)
		}
	}
	got := ""
	for len(got) < len(raw) {
		got += <-received
	}
	if got != string(raw) {
		t.Fatalf("input changed: %q", got)
	}
}
