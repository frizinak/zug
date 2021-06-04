package img

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

var ErrNoHandler = errors.New("no handler")

type Handler interface {
	Get(u *url.URL, dir string) (supported bool, temp bool, path string, err error)
}

type Manager struct {
	rw       sync.RWMutex
	handlers []Handler
	temp     []string
	dir      string
	first    bool
}

func NewManager(handlers []Handler, dir string) *Manager {
	if handlers == nil {
		handlers = make([]Handler, 0)
	}

	if dir == "" {
		dir = filepath.Join(os.TempDir(), "zug")
	}

	return &Manager{handlers: handlers, dir: dir, first: true}
}

func (m *Manager) Register(h Handler) {
	m.rw.Lock()
	m.handlers = append(m.handlers, h)
	m.rw.Unlock()
}

func (m *Manager) Do(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	m.rw.RLock()
	handlers := make([]Handler, len(m.handlers))
	copy(handlers, m.handlers)
	m.rw.RUnlock()

	if m.first {
		m.first = false
		os.MkdirAll(m.dir, 0700)
	}

	for _, h := range handlers {
		ok, temp, val, err := h.Get(u, m.dir)
		if err != nil {
			return "", fmt.Errorf("%w: '%s'", err, uri)
		}
		if !ok {
			continue
		}

		if temp {
			m.rw.Lock()
			m.temp = append(m.temp, val)
			m.rw.Unlock()
		}

		return val, nil
	}

	return "", fmt.Errorf("%w: '%s'", ErrNoHandler, uri)
}

func (m *Manager) Cleanup() error {
	m.rw.RLock()
	var gerr error
	for _, f := range m.temp {
		if err := os.Remove(f); err != nil {
			gerr = err
		}
	}
	m.rw.RUnlock()
	m.rw.Lock()
	m.temp = nil
	m.rw.Unlock()

	return gerr
}

var DefaultManager = NewManager([]Handler{HttpH, FileH}, "")
