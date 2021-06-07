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
	"github.com/frizinak/zug/cli"
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
	x     *x.TermWindow

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
	x, _ := x.NewFromEnv()
	a := &app{
		z:     z,
		c:     c,
		x:     x,
		layer: z.Layer("m"),
		stdin: bufio.NewReader(os.Stdin),
		args:  args,

		tick: make(chan bool, 1),
		quit: make(chan error),

		cursorY: -1,
	}

	a.show()
	a.tick <- true
	a.layer.Draw = true
	a.layer.SynchronouslyDraw = true

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
		a.show()
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
	err := a.layer.SetSource(a.args[a.ix])
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

func (a *app) size(force bool) bool {
	ch, term := a.termSize()
	if !ch && !force {
		return false
	}

	rw := term.X - 2
	rh := term.Y

	a.layer.X = (term.X - rw) / 2
	const space = 2
	if rh > 2*space {
		rh -= 2 * space
		a.layer.Y = space
	}
	a.layer.Width, a.layer.Height = rw, rh

	var chr image.Point
	if a.x != nil {
		if c, err := a.x.CharSize(term.X, term.Y); err == nil {
			chr = c
		}

		dims, err := a.layer.Geometry(chr)
		if err == nil {
			a.layer.Y = term.Y - dims.Dy()
			rh = dims.Dy()
		}
	}

	lines := rh - a.emptyLines
	if a.cursorY != -1 {
		a.layer.Y = a.cursorY
	}

	if lines > 0 {
		l := make([]byte, lines)
		for i := range l {
			l[i] = '\n'
		}
		os.Stdout.Write(l)
		a.emptyLines += lines
	}

	if a.layer.Y > term.Y-rh {
		a.layer.Y = term.Y - a.emptyLines
		ch = true
	}
	if a.layer.Y < space {
		a.layer.Y = space
	}

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
			time.Sleep(time.Millisecond * 200)
		}
	}()

	for {
		select {
		case err := <-a.quit:
			return err
		case upd := <-a.tick:
			if err := a.run(upd); err != nil {
				return err
			}
		}
	}
}

func (a *app) run(redraw bool) error {
	redraw = a.size(redraw) || redraw
	if redraw {
		a.layer.QueueDraw()
	}

	return a.z.Render()
}

func main() {
	var ueberzug string
	flag.StringVar(&ueberzug, "b", "", "ueberzug binary")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "no files given")
		return
	}

	uzug := cli.New(cli.Config{
		UeberzugBinary: ueberzug,
		OnError:        perr,
	})

	if err := uzug.Init(); err != nil {
		perr(err)
		os.Exit(1)
	}

	z := zug.New(img.DefaultManager, uzug)
	term := console.Current()
	app := new(z, term, args)

	_ = term.SetRaw()
	sig := make(chan os.Signal, 1)
	done := func() {
		app.Close()
		term.Reset()
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
