package zug

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/frizinak/zug/cli"
	"github.com/frizinak/zug/img"
)

type Zug struct {
	m   *img.Manager
	cli *cli.Agent

	layers map[string]*Layer
	order  []string
	draw   bool
}

func New(m *img.Manager, ueberzug *cli.Agent) *Zug {
	return &Zug{
		m:      m,
		cli:    ueberzug,
		layers: make(map[string]*Layer),
		order:  []string{},
	}
}

func NewDefaults() *Zug {
	return New(
		img.DefaultManager,
		cli.New(cli.Config{
			OnError: func(err error) {
				fmt.Fprintln(os.Stderr, err)
			},
		}),
	)
}

func (z *Zug) Close() error {
	gerr := []string{}

	if err := z.m.Cleanup(); err != nil {
		gerr = append(gerr, err.Error())
	}

	if err := z.cli.Close(); err != nil {
		gerr = append(gerr, err.Error())
	}

	if len(gerr) == 0 {
		return nil
	}

	return errors.New(strings.Join(gerr, "\n"))
}

func (z *Zug) Layer(name string) *Layer {
	if l, ok := z.layers[name]; ok {
		return l
	}

	l := &Layer{m: z.m, AddCmd: cli.Add(name, "", 0, 0)}
	z.layers[name] = l
	z.order = append(z.order, name)
	z.draw = true

	return l
}

func (z *Zug) Render() error {
	for _, l := range z.layers {
		if l.Path == "" {
			continue
		}
		if l.draw {
			l.draw = false
			z.draw = true
			continue
		}

		stat, err := os.Stat(l.Path)
		if err != nil {
			z.draw = true
			continue
		}

		mtime := stat.ModTime().Truncate(time.Millisecond * 200)
		if mtime.After(l.mtime) {
			z.draw = true
		}
		l.mtime = mtime
	}

	if !z.draw {
		return nil
	}
	z.draw = false

	for _, ln := range z.order {
		l := z.layers[ln]
		if l.Path == "" || l.hide {
			if l.shown {
				l.shown = false
				if err := z.cli.Command(cli.Remove(l.ID)); err != nil {
					return err
				}
			}

			continue
		}

		l.shown = true
		if err := z.cli.Command(l.AddCmd); err != nil {
			return err
		}
	}

	return nil
}

type Layer struct {
	m *img.Manager
	cli.AddCmd

	mtime time.Time
	draw  bool
	hide  bool
	shown bool
}

func (l *Layer) Show() { l.setHidden(false) }
func (l *Layer) Hide() { l.setHidden(true) }

func (l *Layer) setHidden(v bool) {
	if l.hide != v {
		l.hide = v
		l.draw = true
	}
}

func (l *Layer) SetSource(uri string) error {
	path, err := l.m.Do(uri)
	if err != nil {
		return err
	}
	l.Path = path

	return nil
}

func (l *Layer) QueueDraw() { l.draw = true }
