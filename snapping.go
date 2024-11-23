package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func (g *Game) getSearchBounds(mouseX, mouseY, pixelRadius int) Bounds {
	// Convert screen coordinates to geographic coordinates with padding
	minLat, minLon := latLngFromPixel(float64(mouseX-pixelRadius), float64(mouseY+pixelRadius), g)
	maxLat, maxLon := latLngFromPixel(float64(mouseX+pixelRadius), float64(mouseY-pixelRadius), g)

	return Bounds{
		MinX: math.Min(minLon, maxLon),
		MinY: math.Min(minLat, maxLat),
		MaxX: math.Max(minLon, maxLon),
		MaxY: math.Max(minLat, maxLat),
	}
}

// Helper function to find nearest vertex:
func (g *Game) findNearestVertex(mouseX, mouseY int) (*Point, bool) {
	mouseLat, mouseLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
	mousePixelX, mousePixelY := latLngToPixel(mouseLat, mouseLon, g.zoom)

	var nearestPoint *Point
	minDist := g.snapThreshold

	bounds := g.getSearchBounds(mouseX, mouseY, int(g.snapThreshold))

	// Search through all layers recursively
	for _, rootLayer := range g.layers {
		WalkLayers(rootLayer, func(layer *Layer) {
			if !layer.IsEffectivelyVisible() {
				return
			}

			// Check points
			points := layer.PointLayer.Index.Search(bounds)
			for _, p := range points {
				point := p.(*Point)
				px, py := latLngToPixel(point.Lat, point.Lon, g.zoom)
				dist := math.Sqrt(math.Pow(px-mousePixelX, 2) + math.Pow(py-mousePixelY, 2))
				if dist < minDist {
					minDist = dist
					nearestPoint = point
				}
			}

			// Check line vertices
			lines := layer.PolylineLayer.Index.Search(bounds)
			for _, l := range lines {
				line := l.(*LineString)
				for i := range line.Points {
					px, py := latLngToPixel(line.Points[i].Lat, line.Points[i].Lon, g.zoom)
					dist := math.Sqrt(math.Pow(px-mousePixelX, 2) + math.Pow(py-mousePixelY, 2))
					if dist < minDist {
						minDist = dist
						nearestPoint = &line.Points[i]
					}
				}
			}

			// Check polygon vertices
			polys := layer.PolygonLayer.Index.Search(bounds)
			for _, p := range polys {
				poly := p.(*Polygon)
				for i := range poly.Points {
					px, py := latLngToPixel(poly.Points[i].Lat, poly.Points[i].Lon, g.zoom)
					dist := math.Sqrt(math.Pow(px-mousePixelX, 2) + math.Pow(py-mousePixelY, 2))
					if dist < minDist {
						minDist = dist
						nearestPoint = &poly.Points[i]
					}
				}
			}
		})
	}

	return nearestPoint, nearestPoint != nil
}

func (g *Game) drawSnapIndicator(screen *ebiten.Image) {
	// Only draw indicator during insertion or vertex editing modes
	isEditingMode := g.insertMode || g.drawingLine || g.drawingPolygon ||
		(g.vertexEditState != nil && (g.vertexEditState.DragState.IsEditing ||
			g.vertexEditState.InsertionDragState.IsEditing))

	if !g.snappingEnabled || g.snapTarget == nil || !isEditingMode {
		return
	}

	// Convert snap target location to screen coordinates
	x, y := latLngToPixel(g.snapTarget.Lat, g.snapTarget.Lon, g.zoom)
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	screenX := x - (centerX - float64(g.ScreenWidth)/2)
	screenY := y - (centerY - float64(g.ScreenHeight)/2)

	// Draw a cyan square around the snap target
	squareSize := float32(8)
	vector.StrokeRect(screen,
		float32(screenX)-squareSize/2,
		float32(screenY)-squareSize/2,
		squareSize,
		squareSize,
		2,
		color.RGBA{0, 255, 255, 255}, // Cyan
		false)
}
