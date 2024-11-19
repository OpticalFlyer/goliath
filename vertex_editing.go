// vertex_editing.go
package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	vertexHandleRadius = 5.0 // Radius of vertex edit handles in pixels
)

// Represents the currently edited geometry
type VertexEditState struct {
	EditingPoint      *Point
	EditingLine       *LineString
	EditingPolygon    *Polygon
	HoveredVertexID   int // Index of currently hovered vertex, -1 if none
	DragState         DragState
	lastFocusedObject interface{} // Track last focused geometry
}

// Add near the top of vertex_editing.go, after the const declaration
type DragState struct {
	IsEditing      bool
	OriginalPoint  Point
	AdjacentPoints []Point // For lines/polygons
}

func (g *Game) findHoveredGeometry(mouseX, mouseY int) {
	// Don't change hover state if we're editing
	if g.vertexEditState != nil && g.vertexEditState.DragState.IsEditing {
		return
	}

	// Initialize vertex edit state if needed
	if g.vertexEditState == nil {
		g.vertexEditState = &VertexEditState{
			HoveredVertexID: -1,
		}
	}

	// Get geographic coordinates of mouse position
	mouseLat, mouseLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

	// First check if we should maintain focus on current line
	if currentLine, ok := g.vertexEditState.lastFocusedObject.(*LineString); ok && g.PolylineLayer.Visible {
		if currentLine.containsPoint(mouseLat, mouseLon, g.zoom) {
			g.vertexEditState.EditingLine = currentLine
			g.vertexEditState.lastFocusedObject = currentLine

			// Check vertices only for the current focused line
			for i, vertex := range currentLine.Points {
				if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.HoveredVertexID = i
					return
				}
			}
			g.vertexEditState.HoveredVertexID = -1
			return
		}
		fmt.Printf("DEBUG: Leaving focused line\n")
		g.vertexEditState.lastFocusedObject = nil
	}

	// Create small bounds around mouse for spatial query
	const pixelBuffer = 5.0
	minLat, minLon := latLngFromPixel(float64(mouseX-pixelBuffer), float64(mouseY+pixelBuffer), g)
	maxLat, maxLon := latLngFromPixel(float64(mouseX+pixelBuffer), float64(mouseY-pixelBuffer), g)

	searchBounds := Bounds{
		MinX: math.Min(minLon, maxLon),
		MinY: math.Min(minLat, maxLat),
		MaxX: math.Max(minLon, maxLon),
		MaxY: math.Max(minLat, maxLat),
	}

	// If last focused object lost focus, do full precedence check

	// First check points
	if g.PointLayer.Visible {
		points := g.PointLayer.Index.Search(searchBounds)
		for _, p := range points {
			point := p.(*Point)
			if point.containsPoint(mouseLat, mouseLon, g.zoom) {
				g.vertexEditState.EditingPoint = point
				g.vertexEditState.EditingLine = nil
				g.vertexEditState.EditingPolygon = nil
				g.vertexEditState.HoveredVertexID = 0 // Single vertex for points
				return
			}
		}
	}

	// Then check lines
	if g.PolylineLayer.Visible {
		lines := g.PolylineLayer.Index.Search(searchBounds)
		for _, l := range lines {
			line := l.(*LineString)
			// First check vertices
			for i, vertex := range line.Points {
				if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.EditingPoint = nil
					g.vertexEditState.EditingLine = line
					g.vertexEditState.EditingPolygon = nil
					g.vertexEditState.HoveredVertexID = i
					return
				}
			}
			// Then check line segments
			if line.containsPoint(mouseLat, mouseLon, g.zoom) {
				g.vertexEditState.EditingPoint = nil
				g.vertexEditState.EditingLine = line
				g.vertexEditState.EditingPolygon = nil
				g.vertexEditState.HoveredVertexID = -1
				g.vertexEditState.lastFocusedObject = line
				return
			}
		}
	}

	// Finally check polygons
	if g.PolygonLayer.Visible {
		polygons := g.PolygonLayer.Index.Search(searchBounds)
		for _, p := range polygons {
			polygon := p.(*Polygon)
			// First check vertices
			for i, vertex := range polygon.Points {
				if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.EditingPoint = nil
					g.vertexEditState.EditingLine = nil
					g.vertexEditState.EditingPolygon = polygon
					g.vertexEditState.HoveredVertexID = i
					return
				}
			}
			// Then check polygon area
			if polygon.containsPoint(mouseLat, mouseLon, g.zoom) {
				g.vertexEditState.EditingPoint = nil
				g.vertexEditState.EditingLine = nil
				g.vertexEditState.EditingPolygon = polygon
				g.vertexEditState.HoveredVertexID = -1
				return
			}
		}
	}

	// If we get here, nothing was hovered
	g.vertexEditState.EditingPoint = nil
	g.vertexEditState.EditingLine = nil
	g.vertexEditState.EditingPolygon = nil
	g.vertexEditState.HoveredVertexID = -1
}

func (g *Game) drawVertexHandles(screen *ebiten.Image) {
	if g.vertexEditState == nil {
		return
	}

	// Helper function to draw a vertex handle
	drawHandle := func(point Point, isHovered bool) {
		x, y := latLngToPixel(point.Lat, point.Lon, g.zoom)
		centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
		screenX := x - (centerX - float64(g.ScreenWidth)/2)
		screenY := y - (centerY - float64(g.ScreenHeight)/2)

		handleColor := color.RGBA{255, 0, 0, 255} // Red
		if isHovered {
			handleColor = color.RGBA{255, 128, 0, 255} // Orange when hovered
		}

		vector.StrokeCircle(screen,
			float32(screenX), float32(screenY),
			vertexHandleRadius,
			2, handleColor, false)
	}

	// Draw handles based on what type of geometry is being edited
	if g.vertexEditState.EditingPoint != nil {
		drawHandle(*g.vertexEditState.EditingPoint,
			g.vertexEditState.HoveredVertexID == 0)
	} else if g.vertexEditState.EditingLine != nil {
		for i, point := range g.vertexEditState.EditingLine.Points {
			drawHandle(point, g.vertexEditState.HoveredVertexID == i)
		}
	} else if g.vertexEditState.EditingPolygon != nil {
		for i, point := range g.vertexEditState.EditingPolygon.Points {
			drawHandle(point, g.vertexEditState.HoveredVertexID == i)
		}
	}
}

func (g *Game) startVertexDrag() {
	if g.vertexEditState == nil || g.vertexEditState.HoveredVertexID < 0 {
		return
	}

	// Get the original vertex and adjacent vertices
	var origPoint Point
	var adjPoints []Point

	if g.vertexEditState.EditingPoint != nil {
		origPoint = *g.vertexEditState.EditingPoint
	} else if g.vertexEditState.EditingLine != nil {
		points := g.vertexEditState.EditingLine.Points
		idx := g.vertexEditState.HoveredVertexID
		origPoint = points[idx]

		if idx > 0 {
			adjPoints = append(adjPoints, points[idx-1])
		}
		if idx < len(points)-1 {
			adjPoints = append(adjPoints, points[idx+1])
		}
	} else if g.vertexEditState.EditingPolygon != nil {
		points := g.vertexEditState.EditingPolygon.Points
		idx := g.vertexEditState.HoveredVertexID
		origPoint = points[idx]

		prev := (idx - 1 + len(points)) % len(points)
		next := (idx + 1) % len(points)
		adjPoints = append(adjPoints, points[prev], points[next])
	}

	g.vertexEditState.DragState = DragState{
		IsEditing:      true,
		OriginalPoint:  origPoint,
		AdjacentPoints: adjPoints,
	}
}

func (g *Game) drawDragPreview(screen *ebiten.Image) {
	// Only draw preview if we're in editing mode
	if g.vertexEditState == nil || !g.vertexEditState.DragState.IsEditing {
		return
	}

	mouseX, mouseY := ebiten.CursorPosition()
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

	if g.vertexEditState.EditingPoint != nil {
		// Draw X at cursor for points
		crossSize := float32(5)
		mx := float32(mouseX)
		my := float32(mouseY)

		// Draw red X
		vector.StrokeLine(screen,
			mx-crossSize, my-crossSize,
			mx+crossSize, my+crossSize,
			1, color.RGBA{255, 0, 0, 255}, false)
		vector.StrokeLine(screen,
			mx-crossSize, my+crossSize,
			mx+crossSize, my-crossSize,
			1, color.RGBA{255, 0, 0, 255}, false)

		// Draw line from original point to cursor
		origX, origY := latLngToPixel(g.vertexEditState.DragState.OriginalPoint.Lat,
			g.vertexEditState.DragState.OriginalPoint.Lon,
			g.zoom)
		screenOrigX := origX - (centerX - float64(g.ScreenWidth)/2)
		screenOrigY := origY - (centerY - float64(g.ScreenHeight)/2)

		vector.StrokeLine(screen,
			float32(screenOrigX), float32(screenOrigY),
			float32(mouseX), float32(mouseY),
			1, color.RGBA{255, 0, 0, 255}, false)
	} else {
		// Draw lines to adjacent vertices for lines/polygons
		for _, adjPoint := range g.vertexEditState.DragState.AdjacentPoints {
			x, y := latLngToPixel(adjPoint.Lat, adjPoint.Lon, g.zoom)
			screenX := x - (centerX - float64(g.ScreenWidth)/2)
			screenY := y - (centerY - float64(g.ScreenHeight)/2)

			vector.StrokeLine(screen,
				float32(mouseX), float32(mouseY),
				float32(screenX), float32(screenY),
				1, color.RGBA{255, 0, 0, 255}, false)
		}
	}
}

func (g *Game) finishVertexEdit(mouseX, mouseY int) {
	if g.vertexEditState == nil || !g.vertexEditState.DragState.IsEditing {
		return
	}

	// Get new position, possibly snapped
	var newLat, newLon float64
	if g.snappingEnabled {
		if target, found := g.findNearestVertex(mouseX, mouseY); found {
			newLat, newLon = target.Lat, target.Lon
		} else {
			newLat, newLon = latLngFromPixel(float64(mouseX), float64(mouseY), g)
		}
	} else {
		newLat, newLon = latLngFromPixel(float64(mouseX), float64(mouseY), g)
	}

	// Update geometry based on type
	if g.vertexEditState.EditingPoint != nil {
		point := g.vertexEditState.EditingPoint

		updatePoint := func() {
			g.PointLayer.Index.mu.Lock()
			defer g.PointLayer.Index.mu.Unlock()

			// Clear tiles for old position
			oldPoint := Point{
				Lat: g.vertexEditState.DragState.OriginalPoint.Lat,
				Lon: g.vertexEditState.DragState.OriginalPoint.Lon,
			}
			g.clearAffectedTiles(&oldPoint)

			// Remove from R-tree
			g.PointLayer.Index.removeUnsafe(point, point.Bounds())

			// Update position
			point.Lat = newLat
			point.Lon = newLon

			// Re-insert with new bounds
			g.PointLayer.Index.insertUnsafe(point, point.Bounds())

			// Clear tiles for new position
			g.clearAffectedTiles(point)
		}

		// Execute the update
		updatePoint()

	} else if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine

		updateLine := func() {
			g.PolylineLayer.Index.mu.Lock()
			defer g.PolylineLayer.Index.mu.Unlock()

			// Clear tiles for old line position
			oldLine := LineString{
				Points: make([]Point, len(line.Points)),
			}
			copy(oldLine.Points, line.Points)
			g.clearAffectedLineTiles(&oldLine)

			g.PolylineLayer.Index.removeUnsafe(line, line.Bounds())
			line.Points[g.vertexEditState.HoveredVertexID].Lat = newLat
			line.Points[g.vertexEditState.HoveredVertexID].Lon = newLon
			g.PolylineLayer.Index.insertUnsafe(line, line.Bounds())

			// Clear tiles for new line position
			g.clearAffectedLineTiles(line)
		}

		updateLine()

	} else if g.vertexEditState.EditingPolygon != nil {
		polygon := g.vertexEditState.EditingPolygon

		updatePolygon := func() {
			g.PolygonLayer.Index.mu.Lock()
			defer g.PolygonLayer.Index.mu.Unlock()

			// Clear tiles for old polygon position
			oldPolygon := Polygon{
				Points: make([]Point, len(polygon.Points)),
			}
			copy(oldPolygon.Points, polygon.Points)
			g.clearAffectedPolygonTiles(&oldPolygon)

			g.PolygonLayer.Index.removeUnsafe(polygon, polygon.Bounds())
			polygon.Points[g.vertexEditState.HoveredVertexID].Lat = newLat
			polygon.Points[g.vertexEditState.HoveredVertexID].Lon = newLon
			g.PolygonLayer.Index.insertUnsafe(polygon, polygon.Bounds())

			// Clear tiles for new polygon position
			g.clearAffectedPolygonTiles(polygon)
		}

		updatePolygon()
	}

	// Reset edit state
	g.vertexEditState.DragState.IsEditing = false
	g.vertexEditState.DragState.AdjacentPoints = nil
	g.needRedraw = true
}
