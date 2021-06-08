package x

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"strconv"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"golang.org/x/image/draw"
)

type TermWindow struct {
	sem sync.RWMutex

	x   *xgb.Conn
	wnd xproto.Window

	windows map[string]*SubWindow
	depth   xproto.DepthInfo
	visual  xproto.VisualInfo
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

// CharSize returns the size of a single character in pixels
// columns and lines are only used in the fallback calculation and can
// be zero to skip this fallback and return an error.
func (t *TermWindow) CharSize(columns, lines int) (image.Point, error) {
	p, err := t.resizeIncrement()
	if err == nil {
		return p, nil
	}

	if columns == 0 || lines == 0 {
		return p, err
	}

	geom, err := t.Geometry()
	if err != nil {
		return p, err
	}

	p.X = geom.Dx() / columns
	p.Y = geom.Dy() / lines
	return p, nil
}

func (t *TermWindow) resizeIncrement() (image.Point, error) {
	i := image.Point{}

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

	i.X = vals[9]
	i.Y = vals[10]
	if i.X <= 0 || i.Y <= 0 {
		return i, errors.New("no valid resize increment set")
	}

	return i, nil
}

//func (t *TermWindow) ImageReader(r io.Reader) error {
//}

func (t *TermWindow) initWindows() {
	if t.depth.Depth != 0 {
		return
	}

	setup := xproto.Setup(t.x)
	screen := setup.DefaultScreen(t.x)
	depth := screen.AllowedDepths[0]
	visual := depth.Visuals[0]
	t.depth = depth
	t.visual = visual
}

func (t *TermWindow) DelWindow(name string) {
	t.sem.RLock()
	w := t.windows[name]
	if w != nil {
		w.Close()
	}
	t.sem.RUnlock()
}

func (t *TermWindow) delWindow(name string) {
	t.sem.Lock()
	delete(t.windows, name)
	t.sem.Unlock()
}

func (t *TermWindow) SubWindow(name string) (*SubWindow, error) {
	t.initWindows()
	t.sem.Lock()
	w, ok := t.windows[name]
	if !ok {
		w = &SubWindow{t: t, name: name, geom: image.Rect(0, 0, 200, 200)}
	}
	err := w.init()
	if err != nil {
		return nil, err
	}

	t.windows[name] = w
	t.sem.Unlock()

	return w, nil
}

func (t *TermWindow) Render() error {
	for {
		evt, err := t.x.WaitForEvent()
		if err != nil {
			return err
		}

		switch evt.(type) {
		case xproto.ExposeEvent:
			t.sem.RLock()
			for _, w := range t.windows {
				if err := w.Render(); err != nil {
					t.sem.RUnlock()
					return err
				}
			}
			t.sem.RUnlock()
		}
	}
}

type SubWindow struct {
	t    *TermWindow
	name string
	wnd  xproto.Window

	created bool
	mapped  bool
	geom    image.Rectangle
	src     *BGRA
	img     *BGRA
	pixmap  xproto.Pixmap
	gc      xproto.Gcontext
	scaler  ScaleMethod
	change  bool
}

func (w *SubWindow) Close() {
	if w.created {
		xproto.DestroyWindow(w.t.x, w.wnd)
	}
	w.t.delWindow(w.name)
}

func (w *SubWindow) init() error {
	wnd, err := xproto.NewWindowId(w.t.x)
	if err != nil {
		return err
	}

	w.wnd = wnd
	return nil
}

func (w *SubWindow) Show() {
	if w.mapped {
		return
	}

	w.mapped = true
	if w.created {
		xproto.MapWindow(w.t.x, w.wnd)
	}
	w.draw()
}

func (w *SubWindow) Hide() {
	if !w.mapped {
		return
	}

	w.mapped = false
	if w.created {
		xproto.UnmapWindow(w.t.x, w.wnd)
	}
}

func (w *SubWindow) Render() error {
	if !w.mapped {
		return nil
	}

	w.draw()
	return nil
}

func (w *SubWindow) SetScaler(s ScaleMethod) {
	w.change = w.scaler != s
	w.scaler = s
	w.draw()
}

func (w *SubWindow) SetImage(img *BGRA) {
	w.src = img
	w.img = nil
	w.change = true
	w.draw()
}

func (w *SubWindow) SetGeometry(r image.Rectangle) {
	if r == w.geom {
		return
	}

	w.change = w.geom.Dx() != r.Dx() || w.geom.Dy() != r.Dy()
	w.geom = r
	w.draw()
}

func (w *SubWindow) Geometry() Geometry {
	_, g := w.geometry()
	return g
}

func (w *SubWindow) geometry() (bool, Geometry) {
	if w.src == nil {
		return false, Geometry{}
	}
	scaler := scalers[w.scaler]
	if scaler == nil {
		panic("invalid scaler")
	}

	in := Geometry{
		Image:  Dimensions{W: w.src.Rect.Dx(), H: w.src.Rect.Dy()},
		Window: Dimensions{W: w.geom.Dx(), H: w.geom.Dy()},
	}

	out := scaler(in)
	return in.Image != out.Image, out
}

func (w *SubWindow) draw() {
	if w.src == nil || !w.mapped {
		return
	}

	if w.change || !w.created {
		change, geom := w.geometry()

		w.change = false

		w.img = w.src
		if change {
			w.img = NewBGRA(image.Rect(0, 0, geom.Image.W, geom.Image.H))
			draw.NearestNeighbor.Scale(
				w.img,
				w.img.Bounds(),
				w.src,
				w.src.Bounds(),
				draw.Over,
				nil,
			)
		}

		if w.created {
			xproto.DestroyWindow(w.t.x, w.wnd)
		}
		xproto.CreateWindow(
			w.t.x,
			w.t.depth.Depth,
			w.wnd,
			w.t.wnd,
			int16(w.geom.Min.X),
			int16(w.geom.Min.Y),
			uint16(geom.Window.W),
			uint16(geom.Window.H),
			0,
			xproto.WindowClassInputOutput,
			w.t.visual.VisualId,
			xproto.CwBackPixel|xproto.CwEventMask,
			[]uint32{0xffffff, xproto.EventMaskExposure},
		).Check()
		w.created = true
		if w.mapped {
			xproto.MapWindow(w.t.x, w.wnd)
		}

		width, height := uint16(w.img.Rect.Dx()), uint16(w.img.Rect.Dy())
		pixmap, err := xproto.NewPixmapId(w.t.x)
		if err != nil {
			panic(err)
		}
		w.pixmap = pixmap

		gc, err := xproto.NewGcontextId(w.t.x)
		if err != nil {
			panic(err)
		}
		w.gc = gc

		xproto.CreatePixmap(
			w.t.x,
			w.t.depth.Depth,
			w.pixmap,
			xproto.Drawable(w.wnd),
			width,
			height,
		).Check()

		xproto.CreateGC(w.t.x, w.gc, xproto.Drawable(w.pixmap), 0, nil)
		w.t.putImage(
			w.img,
			xproto.Drawable(w.pixmap),
			w.gc,
			w.t.depth.Depth,
		)
	}

	xproto.CopyArea(
		w.t.x,
		xproto.Drawable(w.pixmap),
		xproto.Drawable(w.wnd),
		w.gc,
		0,
		0,
		0,
		0,
		uint16(w.geom.Dx()),
		uint16(w.geom.Dy()),
	)
}
