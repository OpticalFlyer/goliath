// points.go
package main

import (
	"fmt"
	"image/color"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	pointSpriteSize = 10 // Size of the point sprite in pixels
)

var (
	pointSprite *ebiten.Image
)

func init() {
	// Create point sprite - white filled circle with black border
	pointSprite = ebiten.NewImage(pointSpriteSize, pointSpriteSize)

	// Draw white fill
	vector.DrawFilledCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, color.White, false)

	// Draw black border
	vector.StrokeCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, 1, color.Black, false)
}

// InitializeTestPoints adds some random points in the continental US
func (g *Game) InitializeTestPoints() {
	// Continental US bounds approximately
	minLat := 26.000000
	maxLat := 47.000000
	minLon := -123.000000
	maxLon := -76.000000

	// Add random points
	for i := 0; i < 1000000; i++ {
		lat := minLat + rand.Float64()*(maxLat-minLat)
		lon := minLon + rand.Float64()*(maxLon-minLon)
		point := &Point{Lat: lat, Lon: lon}
		g.PointLayer.Index.Insert(point, point.Bounds())
		fmt.Printf("Inserted point %d at (lat: %.4f, lon: %.4f) with bounds: %+v\n", i, lat, lon, point.Bounds()) // Debug statement
	}

	// Add debug print
	fmt.Printf("Added %d points to R-tree\n", g.PointLayer.Index.Size)
}

// DrawPoints renders all points in the current view
func (g *Game) DrawPoints(screen *ebiten.Image) {
	layer := g.PointLayer

	// Calculate view bounds
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	topLeftX := centerX - float64(g.ScreenWidth)/2
	topLeftY := centerY - float64(g.ScreenHeight)/2
	bottomRightX := centerX + float64(g.ScreenWidth)/2
	bottomRightY := centerY + float64(g.ScreenHeight)/2

	topLeftLat, topLeftLon := pixelToLatLng(topLeftX, topLeftY, g.zoom)
	bottomRightLat, bottomRightLon := pixelToLatLng(bottomRightX, bottomRightY, g.zoom)

	viewBounds := Bounds{
		MinX: topLeftLon,
		MinY: bottomRightLat,
		MaxX: bottomRightLon,
		MaxY: topLeftLat,
	}

	// Check if redraw needed
	needsRedraw := layer.dirty ||
		!boundsEqual(layer.bounds, viewBounds) ||
		layer.buffer.Bounds().Dx() != g.ScreenWidth ||
		layer.buffer.Bounds().Dy() != g.ScreenHeight

	if needsRedraw {
		// Resize buffer if needed
		if layer.buffer.Bounds().Dx() != g.ScreenWidth ||
			layer.buffer.Bounds().Dy() != g.ScreenHeight {
			layer.buffer = ebiten.NewImage(g.ScreenWidth, g.ScreenHeight)
		}

		// Clear buffer
		layer.buffer.Clear()

		// Draw points to buffer
		points := layer.Index.Search(viewBounds)
		fmt.Printf("Drawing %d points\n", len(points)) // Debug statement
		for _, p := range points {
			point := p.(*Point)
			pixelX, pixelY := latLngToPixel(point.Lat, point.Lon, g.zoom)
			screenX := pixelX - topLeftX - pointSpriteSize/2
			screenY := pixelY - topLeftY - pointSpriteSize/2

			if screenX >= -pointSpriteSize && screenX <= float64(g.ScreenWidth) &&
				screenY >= -pointSpriteSize && screenY <= float64(g.ScreenHeight) {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(screenX, screenY)
				layer.buffer.DrawImage(pointSprite, op)
			}
		}

		// Update state
		layer.dirty = false
		layer.bounds = viewBounds
	}

	// Draw buffer to screen
	screen.DrawImage(layer.buffer, nil)
}

// Add helper function
func boundsEqual(b1, b2 Bounds) bool {
	return b1.MinX == b2.MinX &&
		b1.MinY == b2.MinY &&
		b1.MaxX == b2.MaxX &&
		b1.MaxY == b2.MaxY
}
