package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"io"
	"net"
	"time"
)

type Request struct {
	Op        string `json:"op"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Remember  bool   `json:"remember,omitempty"`
	Visible   bool   `json:"visible,omitempty"`
	Node      string `json:"node,omitempty"`
	NodeID    int    `json:"node_id,omitempty"`
	AutoStart *bool  `json:"autostart,omitempty"`
}
type Response struct {
	OK      bool            `json:"ok"`
	Code    string          `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func Reply(data any, err error) Response {
	if err != nil {
		message := err.Error()
		var ae *apperr.Error
		if errors.As(err, &ae) {
			message = ae.Message
		}
		return Response{Code: apperr.Code(err), Message: message}
	}
	b, _ := json.Marshal(data)
	return Response{OK: true, Data: b}
}
func Call(ctx context.Context, p platform.Paths, r Request, result any) error {
	c, e := Dial(ctx, p)
	if e != nil {
		return apperr.New("STOPPED", "后台进程未运行，请运行 seusc start")
	}
	defer c.Close()
	deadline := time.Now().Add(4 * time.Minute)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	c.SetDeadline(deadline)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()
	if e = json.NewEncoder(c).Encode(r); e != nil {
		return e
	}
	var response Response
	if e = json.NewDecoder(io.LimitReader(c, 2<<20)).Decode(&response); e != nil {
		return e
	}
	if !response.OK {
		return apperr.New(response.Code, response.Message)
	}
	if result != nil && len(response.Data) > 0 {
		return json.Unmarshal(response.Data, result)
	}
	return nil
}
func Serve(ctx context.Context, l net.Listener, handle func(context.Context, Request) Response) error {
	go func() { <-ctx.Done(); l.Close() }()
	slots := make(chan struct{}, 16)
	for {
		c, e := l.Accept()
		if e != nil {
			return e
		}
		select {
		case slots <- struct{}{}:
		default:
			c.Close()
			continue
		}
		go func() {
			defer func() { <-slots }()
			defer c.Close()
			c.SetDeadline(time.Now().Add(4 * time.Minute))
			var r Request
			if e := json.NewDecoder(io.LimitReader(c, 64<<10)).Decode(&r); e != nil {
				return
			}
			_ = json.NewEncoder(c).Encode(handle(ctx, r))
		}()
	}
}
