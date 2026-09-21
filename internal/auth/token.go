package auth

import (
	"context"
	"net/http"
	"sync"

	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/webshell"
)

type Credentials struct {
	Token   string
	Cookies []*http.Cookie
}
type Provider func(context.Context) (Credentials, error)

// Manager coalesces concurrent refreshes; an invalidation racing a login cannot restore stale credentials.
type Manager struct {
	mu         sync.Mutex
	cached     Credentials
	generation uint64
	gate       chan struct{}
	provider   Provider
}

func New(provider Provider) *Manager {
	return &Manager{provider: provider, gate: make(chan struct{}, 1)}
}
func (m *Manager) Get(ctx context.Context) (string, error) {
	c, e := m.GetCredentials(ctx)
	return c.Token, e
}
func (m *Manager) GetCredentials(ctx context.Context) (Credentials, error) {
	m.mu.Lock()
	c := m.cached
	m.mu.Unlock()
	if c.Token != "" {
		return c, nil
	}
	select {
	case m.gate <- struct{}{}:
	case <-ctx.Done():
		return Credentials{}, ctx.Err()
	}
	defer func() { <-m.gate }()
	m.mu.Lock()
	c = m.cached
	gen := m.generation
	m.mu.Unlock()
	if c.Token != "" {
		return c, nil
	}
	c, err := m.provider(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if c.Token == "" {
		return Credentials{}, errors.New("empty authentication token")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if gen != m.generation {
		return Credentials{}, errors.New("authentication changed during login")
	}
	m.cached = c
	return c, nil
}
func (m *Manager) Invalidate() { m.mu.Lock(); m.cached = Credentials{}; m.generation++; m.mu.Unlock() }
func (m *Manager) InvalidateToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached.Token == token {
		m.cached = Credentials{}
		m.generation++
	}
}
func (m *Manager) Refresh(ctx context.Context) (string, error) { m.Invalidate(); return m.Get(ctx) }
func (m *Manager) HasToken() bool                              { m.mu.Lock(); defer m.mu.Unlock(); return m.cached.Token != "" }
func (m *Manager) Connect(ctx context.Context, client *webshell.Client, node, rows, cols int) (webshell.Session, error) {
	for attempt := 0; attempt < 2; attempt++ {
		c, err := m.GetCredentials(ctx)
		if err != nil {
			return nil, err
		}
		s, err := client.Connect(ctx, webshell.ConnectOptions{Token: c.Token, Cookies: c.Cookies, NodeID: node, Rows: rows, Cols: cols})
		if !errors.Is(err, webshell.ErrUnauthorized) {
			return s, err
		}
		m.InvalidateToken(c.Token)
	}
	return nil, webshell.ErrUnauthorized
}
