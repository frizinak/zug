package x

import (
	"image"
)

// These functions only work for identical images (same Bounds) for a slight
// performance increase.

func YCbCrCopy(dst *BGRA, src *image.YCbCr, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			yo := src.YOffset(x, y)
			co := src.COffset(x, y)
			o := dst.PixOffset(x, y)
			_y := float32(src.Y[yo])
			cr := float32(src.Cr[co]) - 128
			cb := float32(src.Cb[co]) - 128
			r := uint8(floatMinMax(_y+1.40200*cr, 0, 255))
			g := uint8(floatMinMax(_y-0.34414*cb-0.71414*cr, 0, 255))
			b := uint8(floatMinMax(_y+1.77200*cb, 0, 255))
			dst.Pix[o+0] = b
			dst.Pix[o+1] = g
			dst.Pix[o+2] = r
			dst.Pix[o+3] = 255
		}
	}
}

func GrayCopy(dst *BGRA, src *image.Gray, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			v := src.Pix[src.PixOffset(x, y)]
			dst.Pix[o+0] = v
			dst.Pix[o+1] = v
			dst.Pix[o+2] = v
			dst.Pix[o+3] = 255
		}
	}
}

func Gray16Copy(dst *BGRA, src *image.Gray16, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			v := src.Pix[src.PixOffset(x, y)]
			dst.Pix[o+0] = v
			dst.Pix[o+1] = v
			dst.Pix[o+2] = v
			dst.Pix[o+3] = 255
		}
	}
}

func RGBACopy(dst *BGRA, src *image.RGBA, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			dst.Pix[o+0] = src.Pix[o+2]
			dst.Pix[o+1] = src.Pix[o+1]
			dst.Pix[o+2] = src.Pix[o+0]
			dst.Pix[o+3] = src.Pix[o+3]
		}
	}
}

func NRGBACopy(dst *BGRA, src *image.NRGBA, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			a := uint32(src.Pix[o+3])
			dst.Pix[o+0] = uint8(a * (uint32(src.Pix[o+2]) << 8) / 0xffff)
			dst.Pix[o+1] = uint8(a * (uint32(src.Pix[o+1]) << 8) / 0xffff)
			dst.Pix[o+2] = uint8(a * (uint32(src.Pix[o+0]) << 8) / 0xffff)
			dst.Pix[o+3] = src.Pix[o+3]
		}
	}
}

func RGBA64Copy(dst *BGRA, src *image.RGBA64, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			d := src.PixOffset(x, y)
			dst.Pix[o+0] = src.Pix[d+4]
			dst.Pix[o+1] = src.Pix[d+2]
			dst.Pix[o+2] = src.Pix[d+0]
			dst.Pix[o+3] = src.Pix[d+6]
		}
	}
}

func NRGBA64Copy(dst *BGRA, src *image.NRGBA64, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			o := dst.PixOffset(x, y)
			d := src.PixOffset(x, y)
			a := uint32(src.Pix[d+6])

			dst.Pix[o+0] = uint8(a * (uint32(src.Pix[d+4]) << 8) / 0xffff)
			dst.Pix[o+1] = uint8(a * (uint32(src.Pix[d+2]) << 8) / 0xffff)
			dst.Pix[o+2] = uint8(a * (uint32(src.Pix[d+0]) << 8) / 0xffff)

			dst.Pix[o+3] = src.Pix[d+6]
		}
	}
}
