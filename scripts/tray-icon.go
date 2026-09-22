//go:build ignore

// Generates the transparent monochrome template image used by the macOS menu
// bar. Run from the repository root: go run scripts/tray-icon.go
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func drawDisc(dst *image.NRGBA, centerX, centerY, radius float64) {
	minX := int(math.Floor(centerX - radius))
	maxX := int(math.Ceil(centerX + radius))
	minY := int(math.Floor(centerY - radius))
	maxY := int(math.Ceil(centerY + radius))
	radiusSquared := radius * radius
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx := float64(x) + 0.5 - centerX
			dy := float64(y) + 0.5 - centerY
			if dx*dx+dy*dy <= radiusSquared {
				dst.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
}

func drawStroke(dst *image.NRGBA, x0, y0, x1, y1, radius float64) {
	steps := int(math.Ceil(math.Hypot(x1-x0, y1-y0) * 2))
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		drawDisc(dst, x0+(x1-x0)*t, y0+(y1-y0)*t, radius)
	}
}

func downsample(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := src.Bounds().Dx() / size
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var alpha uint32
			for sourceY := y * scale; sourceY < (y+1)*scale; sourceY++ {
				for sourceX := x * scale; sourceX < (x+1)*scale; sourceX++ {
					alpha += uint32(src.NRGBAAt(sourceX, sourceY).A)
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{A: uint8(alpha / uint32(scale*scale))})
		}
	}
	return dst
}

func main() {
	// Draw at 4x and reduce to a 32 px Retina source. AppKit uses only this
	// alpha silhouette and supplies the correct color for the current theme.
	canvas := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for step := 0; step <= 160; step++ {
		t := float64(step) / 160
		x := (1-t)*(1-t)*24 + 2*(1-t)*t*64 + t*t*104
		y := (1-t)*(1-t)*70 + 2*(1-t)*t*20 + t*t*70
		drawDisc(canvas, x, y, 9)
	}
	drawStroke(canvas, 24, 66, 24, 104, 9)
	drawStroke(canvas, 104, 66, 104, 104, 9)
	drawStroke(canvas, 52, 57, 69, 72, 6)
	drawStroke(canvas, 69, 72, 52, 87, 6)

	var output bytes.Buffer
	if err := png.Encode(&output, downsample(canvas, 32)); err != nil {
		panic(err)
	}
	if err := os.WriteFile("frontend/assets/tray-icon.png", output.Bytes(), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("Generated 32 px macOS template tray icon")
}
