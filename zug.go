package zug

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

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
	cli.AddCmd

	m *img.Manager

	mtime time.Time
	draw  bool
	hide  bool
	shown bool

	state struct {
		path string
		dims image.Point
	}
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

// Dimensions returns the original image dimension in pixels
func (l *Layer) Dimensions() (image.Point, error) {
	var i image.Point
	if l.Path == "" {
		return i, nil
	}

	if l.state.path == l.Path {
		return l.state.dims, nil
	}

	r, err := os.Open(l.Path)
	if err != nil {
		return i, err
	}
	defer r.Close()

	dim, _, err := image.DecodeConfig(r)
	i.X = dim.Width
	i.Y = dim.Height

	l.state.path = l.Path
	l.state.dims = i

	return i, err
}

// Geometry returns the position and size of the layer in pixels after scaling
func (l *Layer) GeometryPx(charSize image.Point) (image.Rectangle, error) {
	var i image.Rectangle
	scale, ok := scalers[l.Scaler]
	if !ok {
		return i, fmt.Errorf("invalid scaler '%s'", l.Scaler)
	}

	dim, err := l.Dimensions()
	if err != nil {
		return i, err
	}

	i.Min.X, i.Min.Y = l.X*charSize.X, l.Y*charSize.Y
	w, h := dim.X/charSize.X, dim.Y/charSize.Y
	rw, rh := scale(w, h, l.Width, l.Height)
	i.Max.X, i.Max.Y = i.Min.X+rw*charSize.X, i.Min.Y+rh*charSize.Y

	return i, nil
}

// Geometry returns the position and size of the layer in characters after scaling
func (l *Layer) Geometry(charSize image.Point) (image.Rectangle, error) {
	i, err := l.GeometryPx(charSize)
	if err != nil {
		return i, err
	}

	i.Min.X /= charSize.X
	i.Max.X /= charSize.X
	i.Min.Y /= charSize.Y
	i.Max.Y /= charSize.Y

	return i, err
}

func (l *Layer) QueueDraw() { l.draw = true }
