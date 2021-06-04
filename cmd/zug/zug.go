package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/containerd/console"
	"github.com/frizinak/zug"
	"github.com/frizinak/zug/cli"
	"github.com/frizinak/zug/img"
)

func size(term console.Console, w, h *int) bool {
	termsize, err := term.Size()
	if err != nil {
		return false
	}
	_w, _h := int(termsize.Width), int(termsize.Height)
	c := *w != _w || *h != _h
	*w, *h = _w, _h
	return c
}

func perr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
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
		return
	}

	z := zug.New(img.DefaultManager, uzug)
	term := console.Current()
	_ = term.SetRaw()

	emptyLines := 0
	bye := func() {
		term.Reset()
		fmt.Print("\033[0E")
		for i := 0; i < emptyLines; i++ {
			fmt.Print("\033[K\033[1A")
		}
		z.Close()
	}

	exit := func(err error) {
		if err != nil {
			bye()
			perr(err)
			os.Exit(1)
		}
	}

	defer bye()
	sig := make(chan os.Signal, 1)
	go func() {
		<-sig
		bye()
		os.Exit(0)
	}()
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT)

	layer := z.Layer("m")
	w, h := 20, 5

	setSize := func() {
		if size(term, &w, &h) {
			rh := h
			if rh > 3 {
				rh -= 3
				layer.Y = 3
			}
			for n := emptyLines; n < rh; n++ {
				emptyLines++
				fmt.Println()
			}
			layer.Width, layer.Height = w, rh
			layer.QueueDraw()
		}
	}
	setSize()
	go func() {
		for {
			setSize()
			time.Sleep(time.Millisecond * 200)
		}
	}()

	lix := -1
	ix := 0
	show := func() {
		if ix == lix {
			return
		}
		lix = ix
		if err := layer.SetSource(args[ix]); err != nil {
			perr(err)
			return
		}
		layer.QueueDraw()
	}
	show()

	ex := make(chan error)
	go func() {
		for {
			if err := z.Render(); err != nil {
				ex <- err
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
	}()

	go func() {
		input := bufio.NewReader(os.Stdin)
		for {
			n, err := input.ReadByte()
			exit(err)

			switch n {
			case 3, 'q':
				sig <- syscall.SIGHUP
			case 'j', ' ':
				ix++
				if ix >= len(args) {
					ix = len(args) - 1
				}
			case 'k':
				ix--
				if ix < 0 {
					ix = 0
				}
			}

			show()
		}
	}()

	perr(<-ex)
}
