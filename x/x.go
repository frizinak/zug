package x

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"strconv"
	"sync"

	"github.com/containerd/console"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type TermWindow struct {
	sem sync.RWMutex

	x   *xgb.Conn
	wnd xproto.Window

	console console.Console

	windows map[string]*SubWindow

	depth  xproto.DepthInfo
	visual xproto.VisualInfo
}

func NewFromEnv() (*TermWindow, error) { return New(os.Getenv("WINDOWID")) }

func New(windowID string) (*TermWindow, error) {
	x, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}

	wnd, err := strconv.Atoi(windowID)
	if err != nil {
		return nil, fmt.Errorf("'%s' is no a valid X window id", windowID)
	}

	return &TermWindow{
		x:       x,
		wnd:     xproto.Window(wnd),
		windows: make(map[string]*SubWindow),
	}, nil
}

func (t *TermWindow) Close() {
	t.DelAllWindows()
	t.x.Close()
}

func (t *TermWindow) Console() console.Console {
	if t.console != nil {
		return t.console
	}

	t.console = console.Current()
	return t.console
}

func (t *TermWindow) Geometry() (image.Rectangle, error) {
	i := image.Rectangle{}
	d, err := xproto.GetGeometry(t.x, xproto.Drawable(t.wnd)).Reply()
	if err != nil {
		return i, err
	}

	i.Min.X, i.Min.Y = int(d.X), int(d.Y)
	i.Max.X, i.Max.Y = i.Min.X+int(d.Width), i.Min.Y+int(d.Height)

	return i, nil
}

// CharSize returns the size of a single character in pixels.
func (t *TermWindow) CharSize() (Dimensions, error) {
	p, err := t.resizeIncrement()
	if err == nil {
		return p, nil
	}

	size, err := t.Console().Size()
	if err != nil {
		return p, err
	}
	columns, lines := int(size.Width), int(size.Height)

	if columns == 0 || lines == 0 {
		return p, err
	}

	geom, err := t.Geometry()
	if err != nil {
		return p, err
	}

	p.W = geom.Dx() / columns
	p.H = geom.Dy() / lines
	return p, nil
}

func (t *TermWindow) resizeIncrement() (Dimensions, error) {
	i := Dimensions{}

	aname := "WM_NORMAL_HINTS"
	activeAtom, err := xproto.InternAtom(
		t.x,
		true,
		uint16(len(aname)),
		aname,
	).Reply()
	if err != nil {
		return i, err
	}

	reply, err := xproto.GetProperty(
		t.x,
		false,
		t.wnd,
		activeAtom.Atom,
		xproto.GetPropertyTypeAny, 0, (1<<32)-1,
	).Reply()
	if err != nil {
		return i, err
	}

	size := int(reply.Format / 8)
	vals := make([]int, 0, reply.ValueLen)
	for i := 0; i < len(reply.Value); i += size {
		var v int
		switch reply.Format {
		case 16:
			v = int(binary.LittleEndian.Uint16(reply.Value[i:]))
		case 32:
			v = int(binary.LittleEndian.Uint32(reply.Value[i:]))
		case 64:
			v = int(binary.LittleEndian.Uint64(reply.Value[i:]))
		}

		vals = append(vals, v)
	}

	if len(vals) < 11 {
		return i, errors.New("no resize increment in reply")
	}

	i.W = vals[9]
	i.H = vals[10]
	if i.W <= 0 || i.H <= 0 {
		return i, errors.New("no valid resize increment set")
	}

	return i, nil
}
