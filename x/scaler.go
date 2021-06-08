package x

type Dimensions struct {
	W, H int
}

type Geometry struct {
	Image  Dimensions
	Window Dimensions
}

type Scaler func(c Geometry) Geometry

func ScalerContain(upscale bool) Scaler {
	return func(c Geometry) Geometry {
		if !upscale && c.Image.W <= c.Window.W && c.Image.H <= c.Window.H {
			c.Window = c.Image
			return c
		}

		ir := float64(c.Image.W) / float64(c.Image.H)
		cr := float64(c.Window.W) / float64(c.Window.H)
		if ir > cr {
			if upscale || c.Image.W > c.Window.W {
				c.Image.W = c.Window.W
			}
			c.Image.H = int(float64(c.Image.W) / ir)
			c.Window = c.Image
			return c
		}

		if upscale || c.Image.H > c.Window.H {
			c.Image.H = c.Window.H
		}
		c.Image.W = int(float64(c.Image.H) * ir)
		c.Window = c.Image
		return c
	}
}

type ScaleMethod byte

const (
	ScaleRatio ScaleMethod = iota
	ScaleRatioUpscale
)

var scalers = map[ScaleMethod]Scaler{
	ScaleRatio:        ScalerContain(false),
	ScaleRatioUpscale: ScalerContain(true),
}
