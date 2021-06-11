package x

import (
	"image"
	"image/color"
	"io"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Image interface {
	Bounds() image.Rectangle
	Reset()
	Resize(w int, h int)
	BGRA() *BGRA
}

type nativeImage struct {
	in  *BGRA
	out *BGRA
}

func ImageRead(r io.Reader) (Image, error) {
	_img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	return &nativeImage{in: ImageToBGRA(_img)}, nil
}

func (v *nativeImage) Bounds() image.Rectangle { return v.in.Bounds() }

func (n *nativeImage) Reset() {
	n.out = n.in
}

func (n *nativeImage) Resize(w, h int) {
	n.out = NewBGRA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(
		n.out,
		n.out.Bounds(),
		n.in,
		n.in.Bounds(),
		draw.Src,
		nil,
	)
}

func (n *nativeImage) BGRA() *BGRA {
	if n.out == nil {
		return n.in
	}
	return n.out
}

func floatMinMax(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func ImageToBGRA(i image.Image) *BGRA {
	b := i.Bounds()
	img := NewBGRA(b)

	// Fast path
	switch v := i.(type) {
	case *image.RGBA:
		RGBACopy(img, v, b)
		return img
	case *image.NRGBA:
		NRGBACopy(img, v, b)
		return img

	case *image.RGBA64:
		RGBA64Copy(img, v, b)
		return img
	case *image.NRGBA64:
		NRGBA64Copy(img, v, b)
		return img

	case *image.Gray:
		GrayCopy(img, v, b)
		return img
	case *image.Gray16:
		Gray16Copy(img, v, b)
		return img

	case *image.YCbCr:
		YCbCrCopy(img, v, b)
		return img
	}

	// Slow path
	draw.Draw(img, b, i, image.Point{}, draw.Src)
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
