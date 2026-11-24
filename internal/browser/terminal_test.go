package browser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"github.com/coder/websocket"
)

// Exercise a real browser against a local stand-in for the node picker. Only
// the web page knows how to create the socket and obtain the test cookie.
func TestPageOwnedTerminal(t *testing.T) {
	path, err := Find("")
	if err != nil {
		t.Skip("Chromium not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	b := &Controller{Lifetime: ctx}
	defer b.Reset()
	_, closeLogin, err := b.loginTab(ctx, path, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	closeLogin()
	mux := http.NewServeMux()
	mux.HandleFunc("/webssh-connect", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "test", Path: "/", HttpOnly: true})
		io.WriteString(w, `<script>function openNode(id) { window.testSocket = new WebSocket('ws://' + location.host + '/finder/v2/webshell?NodeId=' + id) };openNode(6)</script><button class="webssh-list-item" onclick="openNode(7)">login02</button><button class="webssh-list-item" onclick="openNode(8)">blocked</button>`)
	})
	mux.HandleFunc("/finder/v2/webshell", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err != nil || c.Value != "test" || r.URL.Query().Get("NodeId") == "8" {
			w.WriteHeader(403)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		resized := false
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var msg struct {
				Type, Data string
				Rows, Cols int
			}
			if json.Unmarshal(data, &msg) != nil {
				return
			}
			if msg.Type == "resize" {
				resized = msg.Rows == 40 && msg.Cols == 120
			}
			if msg.Type == "input" {
				if !resized || r.URL.Query().Get("NodeId") != "7" {
					t.Error("input did not reach the selected, resized terminal")
				}
				conn.Write(r.Context(), websocket.MessageText, []byte("heartbeat:ok"))
				conn.Write(r.Context(), websocket.MessageBinary, []byte(msg.Data))
				conn.Close(websocket.StatusNormalClosure, "done")
				return
			}
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := TerminalClient{Browser: b, BaseURL: srv.URL, NodeName: "login02", InputMode: "json"}
	opts := webshell.ConnectOptions{NodeID: 7, Rows: 40, Cols: 120}
	s, err := client.Connect(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := "中文\x03\x1b[A"
	if _, err = s.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(s)
	if err != nil || string(got) != want {
		t.Fatalf("terminal read %q, %v", got, err)
	}
	client.NodeName = "blocked"
	opts.NodeID = 8
	_, err = client.Connect(ctx, opts)
	if apperr.Code(err) != "WEBSHELL_FORBIDDEN" {
		t.Fatalf("rejected page handshake: %v", err)
	}
}
