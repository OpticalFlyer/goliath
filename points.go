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
	pointSpriteSize = 16 // Size of the point sprite in pixels
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

	// Add 100 random points
	for i := 0; i < 100; i++ {
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
	// Get the center of the map in global pixel coordinates
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

	// Calculate top-left pixel coordinates based on window size
	topLeftX := centerX - float64(g.ScreenWidth)/2
	topLeftY := centerY - float64(g.ScreenHeight)/2

	// Calculate bottom-right pixel coordinates based on window size
	bottomRightX := centerX + float64(g.ScreenWidth)/2
	bottomRightY := centerY + float64(g.ScreenHeight)/2

	// Convert screen bounds to lat/lon
	topLeftLat, topLeftLon := pixelToLatLng(topLeftX, topLeftY, g.zoom)
	bottomRightLat, bottomRightLon := pixelToLatLng(bottomRightX, bottomRightY, g.zoom)

	// Search for points in view
	viewBounds := Bounds{
		MinX: topLeftLon,
		MinY: bottomRightLat, // Note: Y is inverted
		MaxX: bottomRightLon,
		MaxY: topLeftLat,
	}

	fmt.Printf("View Bounds: %+v\n", viewBounds) // Log view bounds

	points := g.PointLayer.Index.Search(viewBounds)
	fmt.Printf("Drawing %d points\n", len(points)) // Debug statement

	// Draw each point
	for _, p := range points {
		point := p.(*Point)

		// Convert point coordinates to global pixel coordinates
		pixelX, pixelY := latLngToPixel(point.Lat, point.Lon, g.zoom)

		// Calculate screen coordinates
		screenX := pixelX - topLeftX
		screenY := pixelY - topLeftY

		// Center the sprite on the point
		x := screenX - pointSpriteSize/2
		y := screenY - pointSpriteSize/2

		// Log point positions
		//fmt.Printf("Point at (lat: %.4f, lon: %.4f) -> (x: %.2f, y: %.2f)\n", point.Lat, point.Lon, x, y)

		// Draw only if point is on screen
		if x >= -pointSpriteSize && x <= float64(g.ScreenWidth) &&
			y >= -pointSpriteSize && y <= float64(g.ScreenHeight) {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(x, y)
			screen.DrawImage(pointSprite, op)
		}
	}
}
