package webshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/coder/websocket"
)

var ErrUnauthorized = errors.New("WebShell authentication rejected")

type ConnectOptions struct {
	Token              string
	Cookies            []*http.Cookie
	NodeID, Rows, Cols int
}
type Session interface {
	io.ReadWriteCloser
	Resize(context.Context, int, int) error
}
type Client struct {
	BaseURL    string
	InputMode  string
	HTTPClient *http.Client
}

func URL(base string, o ConnectOptions) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid WebShell base URL")
	}
	if o.NodeID < 1 || o.Rows < 1 || o.Cols < 1 || o.Rows > 65535 || o.Cols > 65535 {
		return "", errors.New("invalid node or terminal dimensions")
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/finder/v2/webshell"
	u.RawPath = ""
	u.Fragment = ""
	u.User = nil
	q := url.Values{}
	q.Set("Rows", strconv.Itoa(o.Rows))
	q.Set("Cols", strconv.Itoa(o.Cols))
	q.Set("NodeId", strconv.Itoa(o.NodeID))
	q.Set("Authorization", "Bearer "+o.Token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (c *Client) Connect(ctx context.Context, o ConnectOptions) (Session, error) {
	target, err := URL(c.BaseURL, o)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(c.BaseURL)
	h := http.Header{}
	h.Set("Origin", base.Scheme+"://"+base.Host)
	req := &http.Request{Header: h}
	for _, cookie := range o.Cookies {
		req.AddCookie(cookie)
	}
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Never return the dial error: it can contain the credential-bearing URL.
	conn, resp, err := websocket.Dial(dialCtx, target, &websocket.DialOptions{HTTPClient: c.HTTPClient, HTTPHeader: h})
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return nil, ErrUnauthorized
		}
		return nil, apperr.New("WEBSOCKET_FAILED", "无法连接 SEU WebShell，请检查网络和登录节点")
	}
	conn.SetReadLimit(16 << 20)
	return &socket{ctx: ctx, conn: conn, inputMode: c.InputMode}, nil
}

type socket struct {
	inputMode       string
	incomplete      []byte
	ctx             context.Context
	conn            *websocket.Conn
	pending         *bytes.Reader
	readMu, writeMu sync.Mutex
}

func (s *socket) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for {
		if s.pending != nil && s.pending.Len() > 0 {
			return s.pending.Read(p)
		}
		kind, data, err := s.conn.Read(s.ctx)
		if err != nil {
			return 0, err
		}
		if kind != websocket.MessageBinary {
			return 0, apperr.New("WEBSOCKET_FAILED", "WebShell 返回了非终端数据")
		}
		s.pending = bytes.NewReader(data)
	}
}
func (s *socket) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	data := p
	kind := websocket.MessageBinary
	if s.inputMode == "json" {
		data = append(s.incomplete, p...)
		s.incomplete = nil
		end := 0
		for end < len(data) {
			if !utf8.FullRune(data[end:]) {
				s.incomplete = append([]byte(nil), data[end:]...)
				break
			}
			r, n := utf8.DecodeRune(data[end:])
			if r == utf8.RuneError && n == 1 {
				return 0, errors.New("invalid UTF-8 input for JSON WebShell; select binary mode for raw encodings")
			}
			end += n
		}
		if end == 0 {
			return len(p), nil
		}
		var err error
		data, err = json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{"input", string(data[:end])})
		if err != nil {
			return 0, err
		}
		kind = websocket.MessageText
	}
	if err := s.conn.Write(ctx, kind, data); err != nil {
		return 0, err
	}
	return len(p), nil
}
func ResizeMessage(rows, cols int) ([]byte, error) {
	if rows < 1 || cols < 1 || rows > 65535 || cols > 65535 {
		return nil, errors.New("invalid terminal dimensions")
	}
	return json.Marshal(struct {
		Type string `json:"type"`
		Rows int    `json:"rows"`
		Cols int    `json:"cols"`
	}{"resize", rows, cols})
}
func (s *socket) Resize(ctx context.Context, rows, cols int) error {
	b, err := ResizeMessage(rows, cols)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.conn.Write(ctx, websocket.MessageText, b)
}
func (s *socket) Close() error { return s.conn.CloseNow() }
