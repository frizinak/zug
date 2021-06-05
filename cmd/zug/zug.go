package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/containerd/console"
	"github.com/frizinak/zug"
	"github.com/frizinak/zug/cli"
	"github.com/frizinak/zug/img"
)

func perr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
}

var (
	csiBOL = []byte("\033[0E")
	csiCL  = []byte("\033[K")
	csiUP  = []byte("\033[1A")
)

type app struct {
	z     *zug.Zug
	c     console.Console
	layer *zug.Layer
	stdin *bufio.Reader

	args []string
	ix   int

	emptyLines int

	term image.Point

	tick chan bool
	quit chan error
}

func new(z *zug.Zug, c console.Console, args []string) *app {
	a := &app{
		z:     z,
		c:     c,
		layer: z.Layer("m"),
		stdin: bufio.NewReader(os.Stdin),
		args:  args,

		tick: make(chan bool, 1),
		quit: make(chan error),
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

func (a *app) input() {
	n, err := a.stdin.ReadByte()
	if err != nil {
		a.quit <- err
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

func (a *app) size() bool {
	ch, term := a.termSize()
	if !ch {
		return false
	}

	rw := term.X - 2
	rh := term.Y

	a.layer.X = (term.X - rw) / 2
	if rh > 3 {
		rh -= 3
		a.layer.Y = 3
	}

	lines := rh - a.emptyLines
	if lines > 0 {
		l := make([]byte, lines)
		for i := range l {
			l[i] = '\n'
		}
		os.Stdout.Write(l)
		a.emptyLines += lines
	}

	a.layer.Width, a.layer.Height = rw, rh
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
	redraw = a.size() || redraw
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
