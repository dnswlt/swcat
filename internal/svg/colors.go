package svg

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

// AdjustLightness scales the lightness of a hex color by a multiplicative factor.
// A factor > 1.0 brightens the color, while a factor < 1.0 darkens it.
func AdjustLightness(hexStr string, factor float64) (string, error) {
	hexStr = strings.TrimPrefix(hexStr, "#")

	if len(hexStr) == 3 {
		hexStr = string([]byte{
			hexStr[0], hexStr[0],
			hexStr[1], hexStr[1],
			hexStr[2], hexStr[2],
		})
	}

	if len(hexStr) != 6 {
		return "", fmt.Errorf("invalid hex color length: expected 3 or 6 characters, got %d", len(hexStr))
	}

	rgb, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("invalid hex format: %w", err)
	}

	h, s, l := rgbToHsl(rgb[0], rgb[1], rgb[2])

	l = math.Max(0.0, math.Min(1.0, l*factor))

	r, g, b := hslToRgb(h, s, l)

	return fmt.Sprintf("#%02X%02X%02X", r, g, b), nil
}

func rgbToHsl(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	maxV := r
	if g > maxV {
		maxV = g
	}
	if b > maxV {
		maxV = b
	}

	minV := r
	if g < minV {
		minV = g
	}
	if b < minV {
		minV = b
	}

	maxF := float64(maxV) / 255.0
	minF := float64(minV) / 255.0

	l = (maxF + minF) / 2.0

	if maxV == minV {
		return 0, 0, l
	}

	d := maxF - minF
	if l > 0.5 {
		s = d / (2.0 - maxF - minF)
	} else {
		s = d / (maxF + minF)
	}

	switch maxV {
	case r:
		h = (gf - bf) / d
		if g < b {
			h += 6.0
		}
	case g:
		h = (bf-rf)/d + 2.0
	case b:
		h = (rf-gf)/d + 4.0
	}

	h /= 6.0
	return h, s, l
}

func hslToRgb(h, s, l float64) (uint8, uint8, uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255.0))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1.0 + s)
	} else {
		q = l + s - l*s
	}
	p := 2.0*l - q

	r := hueToRgb(p, q, h+1.0/3.0)
	g := hueToRgb(p, q, h)
	b := hueToRgb(p, q, h-1.0/3.0)

	return uint8(math.Round(r * 255.0)), uint8(math.Round(g * 255.0)), uint8(math.Round(b * 255.0))
}

func hueToRgb(p, q, t float64) float64 {
	if t < 0.0 {
		t += 1.0
	}
	if t > 1.0 {
		t -= 1.0
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6.0*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6.0
	}
	return p
}
