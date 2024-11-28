package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawTileGrid draws a grid showing tile boundaries
func (g *Goliath) drawTileGrid(screen *ebiten.Image) {
	// Calculate tile boundaries
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	topLeftX := centerX - float64(g.ScreenWidth)/2
	topLeftY := centerY - float64(g.ScreenHeight)/2

	// Calculate starting tile indices
	startTileX := int(math.Floor(topLeftX / 256))
	startTileY := int(math.Floor(topLeftY / 256))

	// Calculate pixel offsets
	offsetX := math.Mod(topLeftX, 256)
	offsetY := math.Mod(topLeftY, 256)
	if topLeftX < 0 {
		offsetX += 256
		startTileX--
	}
	if topLeftY < 0 {
		offsetY += 256
		startTileY--
	}

	// Draw vertical grid lines
	for x := -offsetX; x <= float64(g.ScreenWidth); x += 256 {
		vector.StrokeLine(screen,
			float32(x), 0,
			float32(x), float32(g.ScreenHeight),
			2, color.RGBA{255, 0, 0, 255}, false)
	}

	// Draw horizontal grid lines
	for y := -offsetY; y <= float64(g.ScreenHeight); y += 256 {
		vector.StrokeLine(screen,
			0, float32(y),
			float32(g.ScreenWidth), float32(y),
			2, color.RGBA{255, 0, 0, 255}, false)
	}
}

func (g *Goliath) drawCenterCrosshair(screen *ebiten.Image) {
	// Calculate screen center
	centerScreenX := float32(g.ScreenWidth / 2)
	centerScreenY := float32(g.ScreenHeight / 2)

	// Size of crosshair lines
	size := float32(20)

	// Draw outer glow (black outline)
	glowColor := color.RGBA{0, 0, 0, 255}
	glowWidth := float32(3)

	// Horizontal line glow
	vector.StrokeLine(screen,
		centerScreenX-size, centerScreenY,
		centerScreenX+size, centerScreenY,
		glowWidth, glowColor, false)

	// Vertical line glow
	vector.StrokeLine(screen,
		centerScreenX, centerScreenY-size,
		centerScreenX, centerScreenY+size,
		glowWidth, glowColor, false)

	// Draw white crosshair
	lineColor := color.RGBA{255, 255, 255, 255}
	lineWidth := float32(1)

	// Horizontal line
	vector.StrokeLine(screen,
		centerScreenX-size, centerScreenY,
		centerScreenX+size, centerScreenY,
		lineWidth, lineColor, false)

	// Vertical line
	vector.StrokeLine(screen,
		centerScreenX, centerScreenY-size,
		centerScreenX, centerScreenY+size,
		lineWidth, lineColor, false)
}

func (g *Goliath) drawLayerBounds(screen *ebiten.Image, layer *Layer) {
	bounds := layer.GetBounds()

	// Convert bounds to screen coordinates
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	minX, minY := latLngToPixel(bounds.MinY, bounds.MinX, g.zoom)
	maxX, maxY := latLngToPixel(bounds.MaxY, bounds.MaxX, g.zoom)

	screenMinX := minX - (centerX - float64(g.ScreenWidth)/2)
	screenMinY := minY - (centerY - float64(g.ScreenHeight)/2)
	screenMaxX := maxX - (centerX - float64(g.ScreenWidth)/2)
	screenMaxY := maxY - (centerY - float64(g.ScreenHeight)/2)

	// Draw the bounds rectangle
	vector.StrokeRect(screen,
		float32(screenMinX),
		float32(screenMinY),
		float32(screenMaxX-screenMinX),
		float32(screenMaxY-screenMinY),
		1,
		color.RGBA{255, 0, 0, 255},
		false,
	)

	// Draw layer name near top-left of bounds
	g.drawText(screen,
		screenMinX+5,
		screenMinY+15,
		color.RGBA{255, 0, 0, 255},
		layer.Name,
	)
}
