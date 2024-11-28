package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawTileGrid draws a grid showing tile boundaries
func (g *Game) drawTileGrid(screen *ebiten.Image) {
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

func (g *Game) drawCenterCrosshair(screen *ebiten.Image) {
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