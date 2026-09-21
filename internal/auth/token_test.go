package auth

import (
	"context"
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentCache(t *testing.T) {
	var calls atomic.Int32
	m := New(func(context.Context) (Credentials, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		return Credentials{Token: "opaque"}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tok, e := m.Get(context.Background()); e != nil || tok != "opaque" {
				t.Error("get failed")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	m.Invalidate()
	_, _ = m.Get(context.Background())
	if calls.Load() != 2 {
		t.Fatal("did not refresh")
	}
}
func TestCancelledWaiter(t *testing.T) {
	gate := make(chan struct{})
	entered := make(chan struct{})
	m := New(func(context.Context) (Credentials, error) {
		close(entered)
		<-gate
		return Credentials{Token: "opaque"}, nil
	})
	go m.Get(context.Background())
	<-entered
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e := m.Get(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(gate)
}
func TestInvalidationRacesLogin(t *testing.T) {
	gate, entered := make(chan struct{}), make(chan struct{})
	m := New(func(context.Context) (Credentials, error) {
		close(entered)
		<-gate
		return Credentials{Token: "stale"}, nil
	})
	done := make(chan error)
	go func() { _, e := m.Get(context.Background()); done <- e }()
	<-entered
	m.Invalidate()
	close(gate)
	if e := <-done; e == nil || m.HasToken() {
		t.Fatal("stale authentication restored")
	}
}
func TestRefreshOnce(t *testing.T) {
	var attempts, providers atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(401)
			return
		}
		c, e := websocket.Accept(w, r, nil)
		if e == nil {
			defer c.CloseNow()
			<-r.Context().Done()
		}
	}))
	defer s.Close()
	m := New(func(context.Context) (Credentials, error) { providers.Add(1); return Credentials{Token: "token"}, nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ws, e := m.Connect(ctx, &webshell.Client{BaseURL: s.URL}, 6, 24, 80)
	if e != nil {
		t.Fatal(e)
	}
	ws.Close()
	cancel()
	if attempts.Load() != 2 || providers.Load() != 2 {
		t.Fatal("retry count incorrect")
	}
}
