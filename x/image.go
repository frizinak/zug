package x

import (
	"image"
	"image/color"
	"image/draw"
	"io"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/jezek/xgb/xproto"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const maxuint16 = 1<<16 - 1

func (t *TermWindow) putImage(
	img *BGRA,
	pixMap xproto.Drawable,
	gc xproto.Gcontext,
	depth byte,
) {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	s := width * 4
	lines := maxuint16 / width
	for y := 0; y < height; y += lines {
		from, till := y*s, (y+lines)*s
		if y+lines > height {
			till = len(img.Pix)
			lines = height - y
		}
		xproto.PutImage(
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

func ImageRead(r io.Reader) (*BGRA, error) {
	_img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	return ImageToBGRA(_img), nil
}

func ImageToBGRA(i image.Image) *BGRA {
	b := i.Bounds()
	img := NewBGRA(b)
	draw.Draw(img, b, i, image.Point{}, draw.Over)
	return img
}

type BGRA struct {
	Rect   image.Rectangle
	Pix    []byte
	Stride int
}

func NewBGRA(r image.Rectangle) *BGRA {
	return &BGRA{
		Rect:   r,
		Pix:    make([]uint8, 4*r.Dx()*r.Dy()),
		Stride: 4 * r.Dx(),
	}
}

func (i *BGRA) ColorModel() color.Model { return color.RGBAModel }
func (i *BGRA) Bounds() image.Rectangle { return i.Rect }

func (i *BGRA) PixOffset(x, y int) int {
	return (y-i.Rect.Min.Y)*i.Stride + (x-i.Rect.Min.X)*4
}

func (i *BGRA) At(x, y int) color.Color {
	if !(image.Point{x, y}.In(i.Rect)) {
		return color.RGBA{}
	}
	o := i.PixOffset(x, y)
	s := i.Pix[o : o+4 : o+4]
	return color.RGBA{s[2], s[1], s[0], s[3]}
}

func (i *BGRA) Set(x, y int, c color.Color) {
	if !(image.Point{x, y}.In(i.Rect)) {
		return
	}
	o := i.PixOffset(x, y)
	c1 := color.RGBAModel.Convert(c).(color.RGBA)
	s := i.Pix[o : o+4 : o+4]
	s[3] = c1.A
	s[2] = c1.R
	s[1] = c1.G
	s[0] = c1.B
}

func (i *BGRA) SubImage(r image.Rectangle) image.Image {
	r = r.Intersect(i.Rect)
	if r.Empty() {
		return &BGRA{Stride: i.Stride}
	}
	o := i.PixOffset(r.Min.X, r.Min.Y)
	return &BGRA{
		Pix:    i.Pix[o:],
		Stride: i.Stride,
		Rect:   r,
	}
}
