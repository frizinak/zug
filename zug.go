package zug

import (
	"errors"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/frizinak/zug/img"
	"github.com/frizinak/zug/x"
)

type Zug struct {
	m    *img.Manager
	term *x.TermWindow

	layers map[string]*Layer
	draw   bool
}

func New(m *img.Manager, term *x.TermWindow) *Zug {
	return &Zug{
		m:      m,
		term:   term,
		layers: make(map[string]*Layer),
	}
}

func NewDefaults() (*Zug, error) {
	term, err := x.NewFromEnv()
	if err != nil {
		return nil, err
	}

	return New(img.DefaultManager, term), nil
}

func (z *Zug) Close() error {
	gerr := []string{}

	if err := z.m.Cleanup(); err != nil {
		gerr = append(gerr, err.Error())
	}

	z.term.Close()

	if len(gerr) == 0 {
		return nil
	}

	return errors.New(strings.Join(gerr, "\n"))
}

func (z *Zug) Layer(name string) *Layer {
	if l, ok := z.layers[name]; ok {
		return l
	}

	wnd := z.term.SubWindow(name)

	l := &Layer{SubWindow: wnd, m: z.m}
	z.layers[name] = l
	z.draw = true

	return l
}

func (z *Zug) Render() error {
	for _, l := range z.layers {
		_ = l.Refresh()
		// TODO log?
	}
	return z.term.Render(false)
}

type Layer struct {
	*x.SubWindow
	m *img.Manager

	state struct {
		path  string
		mtime time.Time
	}
}

// SetImage or SubWindow.SetImage should not be used.
func (l *Layer) SetImage(*x.BGRA) error {
	return errors.New("don't use this directly")
}

// SetSource loads an image from the given URI.
// this might be cached by the file img.Manager.
// Use reload to refresh a local file.
func (l *Layer) SetSource(uri string) error {
	path, err := l.m.Do(uri)
	if err != nil {
		return err
	}
	if path == l.state.path {
		return nil
	}

	if err = l.load(path); err != nil {
		return err
	}

	l.state.path = path
	return nil
}

// Refresh refreshes a local file if mtime has sufficiently changed.
func (l *Layer) Refresh() error {
	if l.state.path == "" {
		return nil
	}

	stat, _ := os.Stat(l.state.path)
	if stat == nil {
		return nil
	}

	mtime := stat.ModTime().Truncate(time.Second)
	if mtime.After(l.state.mtime) {
		return l.load(l.state.path)
	}

	return nil
}

func (l *Layer) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := x.ImageRead(f)
	if err != nil {
		return err
	}

	s, _ := f.Stat()
	l.state.mtime = time.Now()
	if s != nil {
		l.state.mtime = s.ModTime()
	}

	l.SubWindow.SetImage(img)
	return nil
}
