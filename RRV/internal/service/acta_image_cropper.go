package service

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
)

// recortarImagenActa intenta aislar el papel/acta dentro de una foto de camara.
// Si no encuentra un recorte confiable devuelve la imagen original para no romper el flujo.
func recortarImagenActa(imagePath string) (string, bool, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", false, fmt.Errorf("decode image: %w", err)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 300 || h < 300 {
		return imagePath, false, nil
	}

	step := maxInt(1, minInt(w, h)/700)
	sampledCols := (w + step - 1) / step
	sampledRows := (h + step - 1) / step
	colHits := make([]int, sampledCols)
	rowHits := make([]int, sampledRows)

	for sy, y := 0, b.Min.Y; y < b.Max.Y; sy, y = sy+1, y+step {
		for sx, x := 0, b.Min.X; x < b.Max.X; sx, x = sx+1, x+step {
			if pixelParecePapel(img.At(x, y)) {
				rowHits[sy]++
				colHits[sx]++
			}
		}
	}

	rowThreshold := int(math.Max(8, float64(sampledCols)*0.18))
	colThreshold := int(math.Max(8, float64(sampledRows)*0.18))
	y0s, y1s := mayorRangoActivo(rowHits, rowThreshold)
	x0s, x1s := mayorRangoActivo(colHits, colThreshold)
	if y0s < 0 || x0s < 0 {
		return imagePath, false, nil
	}

	x0 := b.Min.X + x0s*step
	y0 := b.Min.Y + y0s*step
	x1 := minInt(b.Min.X+(x1s+1)*step, b.Max.X)
	y1 := minInt(b.Min.Y+(y1s+1)*step, b.Max.Y)

	cropW, cropH := x1-x0, y1-y0
	if cropW < w/3 || cropH < h/3 {
		return imagePath, false, nil
	}
	if cropW > int(float64(w)*0.94) && cropH > int(float64(h)*0.94) {
		return imagePath, false, nil
	}

	padX := int(float64(cropW) * 0.025)
	padY := int(float64(cropH) * 0.025)
	rect := image.Rect(
		maxInt(b.Min.X, x0-padX),
		maxInt(b.Min.Y, y0-padY),
		minInt(b.Max.X, x1+padX),
		minInt(b.Max.Y, y1+padY),
	)

	sub, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return imagePath, false, nil
	}

	tmp, err := os.CreateTemp("", "acta_crop_*.jpg")
	if err != nil {
		return "", false, err
	}
	defer tmp.Close()

	if err := jpeg.Encode(tmp, sub.SubImage(rect), &jpeg.Options{Quality: 94}); err != nil {
		os.Remove(tmp.Name())
		return "", false, err
	}

	return tmp.Name(), true, nil
}

func pixelParecePapel(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
	maxC := math.Max(r8, math.Max(g8, b8))
	minC := math.Min(r8, math.Min(g8, b8))
	gray := r8*0.299 + g8*0.587 + b8*0.114
	sat := maxC - minC

	return (gray > 155 && sat < 75) || gray > 210
}

func mayorRangoActivo(vals []int, threshold int) (int, int) {
	bestStart, bestEnd := -1, -1
	curStart := -1
	for i, v := range vals {
		if v >= threshold {
			if curStart < 0 {
				curStart = i
			}
			continue
		}
		if curStart >= 0 {
			if bestStart < 0 || i-curStart > bestEnd-bestStart+1 {
				bestStart, bestEnd = curStart, i-1
			}
			curStart = -1
		}
	}
	if curStart >= 0 && (bestStart < 0 || len(vals)-curStart > bestEnd-bestStart+1) {
		bestStart, bestEnd = curStart, len(vals)-1
	}
	return bestStart, bestEnd
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
