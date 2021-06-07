package zug

import "github.com/frizinak/zug/cli"

type scaler func(w, h int, canvasW, canvasH int) (int, int)

func scaleContain(w, h, cw, ch int, upscale bool) (int, int) {
	if !upscale && w <= cw && h <= ch {
		return w, h
	}

	ir := float64(w) / float64(h)
	cr := float64(cw) / float64(ch)
	if ir > cr {
		if upscale || w > cw {
			w = cw
		}
		h = int(float64(w) / ir)
		return w, h
	}

	if upscale || h > ch {
		h = ch
	}
	w = int(float64(h) * ir)
	return w, h
}

func scaleExact(w, h, cw, ch int) (int, int) { return cw, ch }

func scaleCrop(w, h, cw, ch int) (int, int) {
	if w > cw {
		w = cw
	}
	if h > ch {
		h = ch
	}
	return w, h
}

var scalers = map[cli.Scaler]scaler{
	cli.Crop:        scaleCrop,
	cli.Distort:     scaleExact,
	cli.ForcedCover: scaleExact,
	cli.FitContain: func(w, h, cw, ch int) (int, int) {
		return scaleContain(w, h, cw, ch, true)
	},
	cli.Contain: func(w, h, cw, ch int) (int, int) {
		return scaleContain(w, h, cw, ch, false)
	},
	cli.Cover: scaleCrop,
}
