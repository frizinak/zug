package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/console"
	"github.com/frizinak/zug"
	"github.com/frizinak/zug/img"
	"github.com/frizinak/zug/x"
)

func perr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
}

var (
	csiBOL    = []byte("\033[0E")
	csiCL     = []byte("\033[K")
	csiUP     = []byte("\033[1A")
	csiCursor = []byte("\033[6n")
)

type app struct {
	z     *zug.Zug
	c     console.Console
	layer *zug.Layer
	stdin *bufio.Reader

	args []string
	ix   int

	escape uint8
	csi    []byte

	emptyLines int

	term    image.Point
	cursorY int

	tick chan bool
	quit chan error
}

const (
	esc uint8 = 1 << iota
	csi
)

func new(z *zug.Zug, c console.Console, args []string) *app {
	a := &app{
		z:     z,
		c:     c,
		layer: z.Layer("m"),
		stdin: bufio.NewReader(os.Stdin),
		args:  args,

		tick: make(chan bool, 1),
		quit: make(chan error),

		cursorY: -1,
	}

	a.show()
	a.tick <- true

	return a
}

func (a *app) next() bool { return a.delta(1) }
func (a *app) prev() bool { return a.delta(-1) }

func (a *app) delta(d int) bool {
	ix := a.ix + d
	if ix >= len(a.args) {
		ix = len(a.args) - 1
	} else if ix < 0 {
		ix = 0
	}
	if ix != a.ix {
		a.ix = ix
		return true
	}

	return false
}

func (a *app) esc(n uint8) bool {
	return a.escape&n != 0
}

func (a *app) input() {
	n, err := a.stdin.ReadByte()
	if err != nil {
		a.quit <- err
		return
	}

	if a.escape&csi != 0 {
		a.csi = append(a.csi, n)
	}

	switch {
	case n == 27 && !a.esc(esc):
		a.escape |= esc
	case n == 91 && !a.esc(csi) && a.esc(esc):
		a.escape |= csi
	case a.esc(esc) && !a.esc(csi) && (n < 0x40 || n > 0x5F):
		a.escape &= ^esc
	case a.esc(csi) && (n >= 0x40 && n <= 0x7E):
		if len(a.csi) != 0 && a.csi[len(a.csi)-1] == 'R' {
			v := strings.Split(string(a.csi), ";")[0]
			if c, err := strconv.Atoi(v); err == nil {
				a.cursorY = c - a.emptyLines
				go func() {
					a.tick <- true
				}()
			}
		}
		a.csi = a.csi[:0]
		a.escape &= ^(esc | csi)
	}

	if a.esc(esc | csi) {
		return
	}

	switch n {
	case 3, 'q':
		a.quit <- nil
	case 'j', ' ':
		if a.next() {
			a.tick <- true
		}
	case 'k':
		if a.prev() {
			a.tick <- true
		}
	}
}

func (a *app) reqCursor() { os.Stdout.Write(csiCursor) }

func (a *app) show() {
	ix := a.ix
	err := a.layer.SetSource(a.args[ix])
	perr(err)
}

func (a *app) termSize() (bool, image.Point) {
	termsize, err := a.c.Size()
	pt := a.term
	if err != nil {
		return false, pt
	}
	w, h := int(termsize.Width), int(termsize.Height)
	c := pt.X != w || pt.Y != h
	pt.X, pt.Y = w, h
	a.term = pt

	return c, pt
}

type dims struct {
	X, Y, W, H int
}

func (d dims) Rect() image.Rectangle {
	return image.Rect(d.X, d.Y, d.X+d.W, d.Y+d.H)
}

func (a *app) size(force bool) bool {
	ch, term := a.termSize()
	if !ch && !force {
		return false
	}

	rw := term.X - 2
	rh := term.Y

	dims := dims{}
	dims.X = (term.X - rw) / 2
	const space = 2
	if rh > 2*space {
		rh -= 2 * space
		dims.Y = space
	}
	dims.W, dims.H = rw, rh

	geomTerm, err := a.layer.DryScaleTerminal(dims.W, dims.H, a.layer.Scaler())
	if err == nil {
		dims.Y = term.Y - geomTerm.Window.H
		rh = geomTerm.Window.H
	}

	lines := rh - a.emptyLines
	if a.cursorY != -1 {
		dims.Y = a.cursorY
	}

	if lines > 0 {
		l := make([]byte, lines)
		for i := range l {
			l[i] = '\n'
		}
		os.Stdout.Write(l)
		a.emptyLines += lines
	}

	if dims.Y > term.Y-rh {
		dims.Y = term.Y - a.emptyLines
		ch = true
	}
	if dims.Y < space {
		dims.Y = space
	}

	_ = a.layer.SetGeometryTerminal(dims.Rect())
	a.layer.Show()
	a.layer.Render()

	if ch {
		a.reqCursor()
	}

	return true
}

func (a *app) Close() {
	b := make([]byte, 0, 1024)
	b = append(b, csiBOL...)
	for i := 0; i < a.emptyLines; i++ {
		b = append(b, csiCL...)
		b = append(b, csiUP...)
	}

	os.Stdout.Write(b)
	a.z.Close()
}

func (a *app) Run() error {
	a.reqCursor()
	go func() {
		for {
			a.input()
		}
	}()
	go func() {
		for {
			a.tick <- false
			time.Sleep(time.Millisecond * 50)
		}
	}()

	update := false
	tick := false
	for {
		select {
		case err := <-a.quit:
			return err
		case upd := <-a.tick:
			tick = true
			update = upd || update
		case <-time.After(time.Millisecond * 10):
			if !tick {
				continue
			}
			if update {
				a.show()
			}
			if err := a.run(update); err != nil {
				return err
			}
			tick = false
			update = false
		}
	}
}

func (a *app) run(redraw bool) error {
	a.size(redraw)
	return a.z.Render()
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "no files given")
		return
	}

	x, err := x.NewFromEnv()
	if err != nil {
		perr(err)
		os.Exit(1)
	}

	z := zug.New(img.DefaultManager, x)
	term := console.Current()
	app := new(z, term, args)

	_ = term.SetRaw()
	sig := make(chan os.Signal, 1)
	done := func() {
		app.Close()
		_ = term.Reset()
		os.Exit(0)
	}
	go func() {
		<-sig
		done()
		os.Exit(0)
	}()
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	if err := app.Run(); err != nil {
		perr(err)
		os.Exit(1)
	}

	done()
}
