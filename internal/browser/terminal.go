package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"github.com/chromedp/cdproto/inspector"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// TerminalClient attaches to the socket created by the official WebSSH page.
// The site owns authentication, node selection, handshake and keepalives.
type TerminalClient struct {
	Browser                      *Controller
	BaseURL, NodeName, InputMode string
}

func (c *TerminalClient) Connect(ctx context.Context, o webshell.ConnectOptions) (webshell.Session, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	c.Browser.mu.Lock()
	root := c.Browser.root
	if root == nil || root.Err() != nil {
		c.Browser.mu.Unlock()
		return nil, webshell.ErrUnauthorized
	}
	tab, closeTab := linkedTab(root, ctx)
	c.Browser.mu.Unlock()
	s := &terminal{ctx: tab, closeTab: closeTab, changed: make(chan struct{}, 1), binary: c.InputMode == "binary"}
	ready := make(chan error, 1)
	var id network.RequestID
	socketScheme := "wss"
	if base.Scheme == "http" {
		socketScheme = "ws"
	}
	nodeID := strconv.Itoa(o.NodeID)
	report := func(e error) {
		select {
		case ready <- e:
		default:
		}
	}
	chromedp.ListenTarget(tab, func(event any) {
		switch v := event.(type) {
		case *inspector.EventDetached, *inspector.EventTargetCrashed:
			report(terminalFailed())
			s.finish(terminalFailed())
		case *network.EventWebSocketCreated:
			u, e := url.Parse(v.URL)
			if id == "" && e == nil && u.Scheme == socketScheme && u.Host == base.Host && u.Path == "/finder/v2/webshell" && u.Query().Get("NodeId") == nodeID {
				id = v.RequestID
			}
		case *network.EventWebSocketHandshakeResponseReceived:
			if v.RequestID == id {
				switch v.Response.Status {
				case 101:
					report(nil)
				case 401:
					report(webshell.ErrUnauthorized)
				case 403:
					report(forbidden())
				default:
					report(terminalFailed())
				}
			}
		case *network.EventWebSocketFrameError:
			if v.RequestID == id {
				e := terminalFailed()
				// Chromium may only report rejected upgrades through the frame error.
				if strings.Contains(v.ErrorMessage, "response code: 403") {
					e = forbidden()
				}
				if strings.Contains(v.ErrorMessage, "response code: 401") {
					e = webshell.ErrUnauthorized
				}
				report(e)
				s.finish(e)
			}
		case *network.EventWebSocketClosed:
			if v.RequestID == id {
				report(terminalFailed())
				s.finish(io.EOF)
			}
		case *network.EventWebSocketFrameReceived:
			if v.RequestID == id {
				s.receive(v.Response)
			}
		}
	})
	success := false
	defer func() {
		if !success {
			s.Close()
		}
	}()
	dial, cancel := context.WithTimeout(tab, 35*time.Second)
	defer cancel()
	// chromedp runs the target event loop on the context used for its first
	// Run. Keep that loop alive after the handshake deadline is released.
	if err := chromedp.Run(tab, network.Enable()); err != nil {
		return nil, terminalFailed()
	}
	if err := chromedp.Run(dial, chromedp.Navigate(base.JoinPath("webssh-connect").String())); err != nil {
		return nil, terminalFailed()
	}
	name, _ := json.Marshal(c.NodeName)
	origin, _ := json.Marshal(base.Scheme + "://" + base.Host)
	// The page opens its first node automatically. Click the requested node
	// once when it is listed and another tab is active.
	selectNode := fmt.Sprintf(`(() => {
		if (location.origin !== %s || document.querySelector("#form_item_password") || /\/login\/?$/.test(location.pathname)) return "login";
		const name = %s;
		const items = Array.from(document.querySelectorAll(".webssh-list-item"));
		if (!items.length) return "";
		const item = items.find(el => el.textContent.trim() === name);
		if (!item) return "missing";
		const active = document.querySelector(".ant-tabs-tab-active .ant-tabs-tab-btn");
		if (active?.textContent.trim() === name) return "";
		item.click();
		return "clicked";
	})()`, origin, name)
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	clicked := false
	for {
		select {
		case e := <-ready:
			if e != nil {
				return nil, e
			}
			if e = s.attach(dial, base.Host, nodeID, socketScheme); e != nil {
				return nil, e
			}
			if e = s.Resize(dial, o.Rows, o.Cols); e != nil {
				return nil, e
			}
			success = true
			return s, nil
		case <-timer.C:
			if clicked {
				continue
			}
			var state string
			if err := chromedp.Run(dial, chromedp.Evaluate(selectNode, &state)); err != nil {
				continue
			}
			switch state {
			case "clicked":
				clicked = true
			case "login":
				return nil, webshell.ErrUnauthorized
			case "missing":
				return nil, apperr.New("NODE_UNAVAILABLE", "官方终端的节点列表中没有配置的登录节点")
			}
		case <-dial.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, terminalFailed()
		}
	}
}

func terminalFailed() error {
	return apperr.New("WEBSOCKET_FAILED", "未能连接官方 WebSSH 页面中的终端")
}
func forbidden() error {
	return apperr.New("WEBSHELL_FORBIDDEN", "官方 WebSSH 页面拒绝了终端连接（HTTP 403）")
}

type terminal struct {
	ctx      context.Context
	closeTab func()
	once     sync.Once
	socket   runtime.RemoteObjectID
	mu       sync.Mutex
	output   bytes.Buffer
	err      error
	changed  chan struct{}
	writeMu  sync.Mutex
	encoder  webshell.InputEncoder
	binary   bool
}

func (s *terminal) attach(ctx context.Context, host, node, scheme string) error {
	// Keep a CDP handle to this single page-owned socket. A reconnect by
	// the page must never redirect SSH input to a replacement shell.
	endpoint, _ := json.Marshal(scheme + "://" + host + "/finder/v2/webshell")
	id, _ := json.Marshal(node)
	find := fmt.Sprintf(`async function() {
		const matches = this.filter(ws => {
			const u = new URL(ws.url);
			return u.origin + u.pathname === %s && u.searchParams.get("NodeId") === %s && ws.readyState <= WebSocket.OPEN;
		});
		if (matches.length !== 1) return null;
		const ws = matches[0];
		if (ws.readyState === WebSocket.CONNECTING) {
			await new Promise((resolve, reject) => {
				ws.addEventListener("open", resolve, {once: true});
				ws.addEventListener("close", reject, {once: true});
			});
		}
		return ws;
	}`, endpoint, id)
	return chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		prototype, exception, e := runtime.Evaluate("WebSocket.prototype").Do(c)
		if e != nil || exception != nil || prototype.ObjectID == "" {
			return terminalFailed()
		}
		sockets, e := runtime.QueryObjects(prototype.ObjectID).Do(c)
		if e != nil {
			return terminalFailed()
		}
		obj, exception, e := runtime.CallFunctionOn(find).WithObjectID(sockets.ObjectID).WithAwaitPromise(true).Do(c)
		if e != nil || exception != nil || obj.ObjectID == "" {
			return terminalFailed()
		}
		s.socket = obj.ObjectID
		return nil
	}))
}

func (s *terminal) signal() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}
func (s *terminal) finish(e error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = e
	}
	s.signal()
}
func (s *terminal) receive(f *network.WebSocketFrame) {
	var data []byte
	switch f.Opcode {
	case 1:
		data = []byte(f.PayloadData)
	case 2:
		var err error
		data, err = base64.StdEncoding.DecodeString(f.PayloadData)
		if err != nil {
			s.finish(terminalFailed())
			return
		}
	default:
		return
	}
	if bytes.Equal(data, []byte("heartbeat:ok")) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return
	}
	if s.output.Len()+len(data) > 16<<20 {
		s.err = errors.New("WebSSH terminal output buffer full")
	} else {
		s.output.Write(data)
	}
	s.signal()
}
func (s *terminal) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		s.mu.Lock()
		if s.output.Len() > 0 {
			n, e := s.output.Read(p)
			s.mu.Unlock()
			return n, e
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return 0, err
		}
		select {
		case <-s.changed:
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		}
	}
}
func (s *terminal) send(ctx context.Context, data []byte, binary bool) error {
	fn := "function(value){if(this.readyState!==1) throw Error();this.send(value)}"
	value := string(data)
	if binary {
		fn = "function(value){if(this.readyState!==1) throw Error();this.send(Uint8Array.from(atob(value), c=>c.charCodeAt(0)))}"
		value = base64.StdEncoding.EncodeToString(data)
	}
	arg, _ := json.Marshal(value)
	send, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	return chromedp.Run(send, chromedp.ActionFunc(func(c context.Context) error {
		_, ex, err := runtime.CallFunctionOn(fn).WithObjectID(s.socket).WithArguments([]*runtime.CallArgument{{Value: arg}}).Do(c)
		if err != nil || ex != nil {
			return terminalFailed()
		}
		return nil
	}))
}
func (s *terminal) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data := p
	if !s.binary {
		var e error
		data, e = s.encoder.Encode(p)
		if e != nil {
			return 0, e
		}
	}
	if len(data) > 0 {
		if e := s.send(s.ctx, data, s.binary); e != nil {
			return 0, e
		}
	}
	return len(p), nil
}
func (s *terminal) Resize(ctx context.Context, rows, cols int) error {
	data, e := webshell.ResizeMessage(rows, cols)
	if e != nil {
		return e
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.send(ctx, data, false)
}
func (s *terminal) Close() error { s.once.Do(func() { s.finish(io.EOF); s.closeTab() }); return nil }
