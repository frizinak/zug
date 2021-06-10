package x

import (
	"errors"
	"image"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

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

// DelWindow will close a subwindow by name. Identical to calling Close on
// the subwindow.
func (t *TermWindow) DelWindow(name string) {
	t.sem.RLock()
	w := t.windows[name]
	if w != nil {
		w.Close()
	}
	t.sem.RUnlock()
}

func (t *TermWindow) DelAllWindows() {
	t.sem.RLock()
	l := make([]string, 0, len(t.windows))
	t.sem.RUnlock()
	for _, name := range l {
		t.DelWindow(name)
	}
}

func (t *TermWindow) delWindow(name string) {
	t.sem.Lock()
	delete(t.windows, name)
	t.sem.Unlock()
}

// Subwindow creates a new subwindow instance if one doesn't already exist
// by that name. Returns the window.
func (t *TermWindow) SubWindow(name string) *SubWindow {
	t.initWindows()
	t.sem.Lock()
	w, ok := t.windows[name]
	if !ok {
		w = &SubWindow{t: t, name: name, geom: image.Rectangle{}}
	}

	t.windows[name] = w
	t.sem.Unlock()

	return w
}

// Render should be called to process X11 ExposeEvents, so subwindows
// are drawn when appropriate. This method blocks until an event is
// received when block is true.
func (t *TermWindow) Render(block bool) error {
	if block {
		evt, err := t.x.WaitForEvent()
		if err != nil {
			return err
		}
		if evt == nil {
			return errors.New("X server connection closed")
		}
		return t.processEvent(evt)
	}

	evt, err := t.x.PollForEvent()
	if err != nil {
		return err
	}

	return t.processEvent(evt)
}

func (t *TermWindow) processEvent(event xgb.Event) error {
	if event == nil {
		return nil
	}
	switch event.(type) {
	case xproto.ExposeEvent:
		t.sem.RLock()
		for _, w := range t.windows {
			w.Render()
		}
		t.sem.RUnlock()
	}

	return nil
}

type state byte

const (
	stateMapped state = 1 << iota
	stateCreated
)

type SubWindow struct {
	sem  sync.Mutex
	t    *TermWindow
	name string
	wnd  xproto.Window

	state  state
	geom   image.Rectangle
	src    Image
	img    *BGRA
	pixmap xproto.Pixmap
	gc     xproto.Gcontext
	scaler ScaleMethod

	change bool
}

// Closes frees resources on the x server and this subwindow must not be
// used anymore.
func (w *SubWindow) Close() {
	w.src = nil

	w.sem.Lock()
	if w.is(stateCreated) {
		xproto.DestroyWindow(w.t.x, w.wnd)
		w.wnd = 0
	}
	if w.pixmap != 0 {
		xproto.FreePixmap(w.t.x, w.pixmap)
	}
	if w.gc != 0 {
		xproto.FreeGC(w.t.x, w.gc)
	}
	w.sem.Unlock()

	w.t.delWindow(w.name)
}

func (w *SubWindow) Show() {
	w.sem.Lock()
	defer w.sem.Unlock()
	if w.is(stateMapped) {
		return
	}

	w.state |= stateMapped
	if w.is(stateCreated) {
		xproto.MapWindow(w.t.x, w.wnd)
	}

	w.draw()
}

func (w *SubWindow) Hide() {
	w.sem.Lock()
	defer w.sem.Unlock()
	if !w.is(stateMapped) {
		return
	}

	w.state &= ^stateMapped
	if w.is(stateCreated) {
		xproto.UnmapWindow(w.t.x, w.wnd)
	}
}

func (w *SubWindow) ToTop() {
	w.sem.Lock()
	defer w.sem.Unlock()
	if !w.is(stateCreated) || !w.is(stateMapped) {
		return
	}

	w.windowID(true)
	w.draw()

	// Does nothing:
	//   xproto.CirculateWindow(w.t.x, xproto.CirculateRaiseLowest, w.wnd)
	// Or
	//   xproto.UnmapWindowChecked(w.t.x, w.wnd).Check()
	//   xproto.MapWindowChecked(w.t.x, w.wnd).Check()
}

func (w *SubWindow) Render() {
	w.sem.Lock()
	defer w.sem.Unlock()
	if !w.is(stateMapped) {
		return
	}

	w.draw()
}

func (w *SubWindow) DryScale(
	width, height int,
	s ScaleMethod,
) (pix Geometry) {
	_, pix = w.calcGeom(width, height, s)
	return
}

func (w *SubWindow) DryScaleTerminal(
	width, height int,
	s ScaleMethod,
) (term Geometry, err error) {
	var chr Dimensions
	chr, err = w.t.CharSize()
	if err != nil {
		return
	}
	_, pix := w.calcGeom(width*chr.W, height*chr.H, s)
	term = w.calcGeomTerminal(chr, pix)
	return
}

func (w *SubWindow) Scaler() ScaleMethod { return w.scaler }

func (w *SubWindow) SetScaler(s ScaleMethod) {
	w.sem.Lock()
	w.change = w.change || w.scaler != s
	w.scaler = s
	w.sem.Unlock()
}

func (w *SubWindow) SetImage(img Image) {
	w.sem.Lock()
	w.src = img
	w.img = nil
	w.change = true
	if w.geom == (image.Rectangle{}) {
		b := w.src.Bounds()
		w.geom = image.Rect(0, 0, b.Dx(), b.Dy())
	}

	w.sem.Unlock()
}

// SetGeometryTerminal in terminal units (columns and lines).
func (w *SubWindow) SetGeometryTerminal(r image.Rectangle) error {
	chr, err := w.t.CharSize()
	if err != nil {
		return err
	}

	width, height := r.Dx(), r.Dy()
	r.Min.X *= chr.W
	r.Min.Y *= chr.H
	r.Max.X = r.Min.X + width*chr.W
	r.Max.Y = r.Min.Y + height*chr.H
	w.SetGeometry(r)

	return nil
}

// SetGeometry in pixels.
func (w *SubWindow) SetGeometry(r image.Rectangle) {
	w.sem.Lock()
	defer w.sem.Unlock()
	if r == w.geom {
		return
	}

	w.change = w.change || w.geom != r
	w.geom = r
}

// GeometryTerminal returns the geometry in terminal units (columns and lines).
func (w *SubWindow) GeometryTerminal() (Geometry, error) {
	chr, err := w.t.CharSize()
	if err != nil {
		return Geometry{}, err
	}

	w.sem.Lock()
	defer w.sem.Unlock()
	_, g := w.geometry()
	return w.calcGeomTerminal(chr, g), nil
}

// Geometry in pixels.
func (w *SubWindow) Geometry() Geometry {
	w.sem.Lock()
	_, g := w.geometry()
	w.sem.Unlock()
	return g
}

func (w *SubWindow) is(s state) bool { return w.state&s != 0 }

func (w *SubWindow) windowID(new bool) {
	if w.wnd != 0 {
		if !new {
			return
		}
		xproto.DestroyWindow(w.t.x, w.wnd)
		w.wnd = 0
	}

	w.state &= ^stateCreated
	wnd, _ := xproto.NewWindowId(w.t.x)
	w.wnd = wnd
}

func (w *SubWindow) calcGeomTerminal(chr Dimensions, g Geometry) Geometry {
	g.Image.W /= chr.W
	g.Image.H /= chr.H
	g.Window.W /= chr.W
	g.Window.H /= chr.H

	return g
}

func (w *SubWindow) calcGeom(width, height int, s ScaleMethod) (in, out Geometry) {
	if w.src == nil {
		return
	}
	scaler := scalers[s]
	if scaler == nil {
		panic("invalid scaler")
	}

	b := w.src.Bounds()
	in = Geometry{
		Image:  Dimensions{W: b.Dx(), H: b.Dy()},
		Window: Dimensions{W: width, H: height},
	}

	out = scaler(in)
	return
}

func (w *SubWindow) geometry() (bool, Geometry) {
	in, out := w.calcGeom(w.geom.Dx(), w.geom.Dy(), w.scaler)
	return in.Image != out.Image, out
}

func (w *SubWindow) drawImage() {
	change, geom := w.geometry()
	renderable := geom.Window.W != 0 && geom.Window.H != 0 &&
		geom.Image.W != 0 && geom.Image.H != 0

	actualChange := w.img == nil
	if renderable && !actualChange {
		b := w.img.Rect
		actualChange = b.Dx() != geom.Image.W || b.Dy() != geom.Image.H
	}

	if renderable && actualChange {
		if change {
			w.src.Resize(geom.Image.W, geom.Image.H)
		}
		w.img = w.src.BGRA()
	}

	if w.is(stateCreated) {
		xproto.DestroyWindow(w.t.x, w.wnd)
		w.wnd = 0
		if actualChange {
			xproto.FreePixmap(w.t.x, w.pixmap)
			xproto.FreeGC(w.t.x, w.gc)
			w.pixmap, w.gc = 0, 0
		}
		w.state &= ^stateCreated
	}

	if !renderable {
		return
	}

	w.windowID(false)
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
	)
	w.state |= stateCreated

	if actualChange || w.pixmap == 0 || w.gc == 0 {
		width, height := uint16(w.img.Rect.Dx()), uint16(w.img.Rect.Dy())
		pixmap, _ := xproto.NewPixmapId(w.t.x)
		gc, _ := xproto.NewGcontextId(w.t.x)

		w.pixmap = pixmap
		w.gc = gc
		xproto.CreatePixmap(
			w.t.x,
			w.t.depth.Depth,
			w.pixmap,
			xproto.Drawable(w.wnd),
			width,
			height,
		)

		xproto.CreateGC(w.t.x, w.gc, xproto.Drawable(w.pixmap), 0, nil)
		w.t.putImage(
			w.img,
			xproto.Drawable(w.pixmap),
			w.gc,
			w.t.depth.Depth,
		)
	}

	if w.is(stateMapped) {
		xproto.MapWindow(w.t.x, w.wnd)
	}
}

func (w *SubWindow) draw() {
	if w.src == nil || !w.is(stateMapped) {
		return
	}

	width, height := uint16(w.geom.Dx()), uint16(w.geom.Dy())
	if width == 0 || height == 0 {
		return
	}

	if w.change || !w.is(stateCreated) {
		w.change = false
		w.drawImage()
	}
	if w.pixmap == 0 {
		return
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
		width,
		height,
	)
}

const maxuint16 = 1<<16 - 1

func (t *TermWindow) putImage(
	img *BGRA,
	pixMap xproto.Drawable,
	gc xproto.Gcontext,
	depth byte,
) {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width == 0 || height == 0 {
		return
	}

	s := width * 4
	lines := maxuint16 / s
	for y := 0; y < height; y += lines {
		from, till := y*s, (y+lines)*s
		if y+lines > height {
			till = len(img.Pix)
			lines = height - y
		}
		xproto.PutImageChecked(
			t.x,
			xproto.ImageFormatZPixmap,
			pixMap,
			gc,
			uint16(width),
			uint16(lines),
			0, int16(y),
			0, depth,
			img.Pix[from:till],
		)
	}
}
