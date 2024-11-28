// vertex_editing.go
package main

import (
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
	EditingPoint       *Point
	EditingLine        *LineString
	EditingPolygon     *Polygon
	HoveredVertexID    int // Index of currently hovered vertex, -1 if none
	DragState          DragState
	lastFocusedObject  interface{} // Track last focused geometry
	HoveredInsertionID int         // Index of line segment being hovered (-1 if none)
	InsertionDragState struct {
		IsEditing         bool
		SegmentStartPoint Point
		SegmentEndPoint   Point
	}
}

// Add near the top of vertex_editing.go, after the const declaration
type DragState struct {
	IsEditing      bool
	OriginalPoint  Point
	AdjacentPoints []Point // For lines/polygons
}

func isNearMidpoint(mouseLat, mouseLon float64, p1, p2 Point, zoom int) bool {
	midLat := (p1.Lat + p2.Lat) / 2
	midLon := (p1.Lon + p2.Lon) / 2

	// Use pixel-based distance check
	const pixelRadius = 5.0
	px1, py1 := latLngToPixel(midLat, midLon, zoom)
	px2, py2 := latLngToPixel(mouseLat, mouseLon, zoom)

	dx := px2 - px1
	dy := py2 - py1
	distanceInPixels := math.Sqrt(dx*dx + dy*dy)

	return distanceInPixels <= pixelRadius
}

func (g *Goliath) findHoveredGeometry(mouseX, mouseY int) {
	// Don't change hover state if we're editing
	if g.vertexEditState != nil && (g.vertexEditState.DragState.IsEditing ||
		g.vertexEditState.InsertionDragState.IsEditing) {
		return
	}

	// Initialize vertex edit state if needed
	if g.vertexEditState == nil {
		g.vertexEditState = &VertexEditState{
			HoveredVertexID:    -1,
			HoveredInsertionID: -1,
		}
	}

	g.vertexEditState.HoveredInsertionID = -1

	// Get geographic coordinates of mouse position
	mouseLat, mouseLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

	// Check insertion points for focused line
	if currentLine, ok := g.vertexEditState.lastFocusedObject.(*LineString); ok {
		WalkLayers(g.layers[0], func(layer *Layer) {
			if !layer.IsEffectivelyVisible() {
				return
			}
			lines := layer.PolylineLayer.Index.Search(currentLine.Bounds())
			for _, l := range lines {
				if l == currentLine {
					for i := 0; i < len(currentLine.Points)-1; i++ {
						if isNearMidpoint(mouseLat, mouseLon, currentLine.Points[i], currentLine.Points[i+1], g.zoom) {
							g.vertexEditState.HoveredInsertionID = i
							return
						}
					}
				}
			}
		})
	}

	// Check insertion points for focused polygon
	if currentPolygon, ok := g.vertexEditState.lastFocusedObject.(*Polygon); ok {
		WalkLayers(g.layers[0], func(layer *Layer) {
			if !layer.IsEffectivelyVisible() {
				return
			}
			polygons := layer.PolygonLayer.Index.Search(currentPolygon.Bounds())
			for _, p := range polygons {
				if p == currentPolygon {
					for i := 0; i < len(currentPolygon.Points); i++ {
						p1 := currentPolygon.Points[i]
						p2 := currentPolygon.Points[(i+1)%len(currentPolygon.Points)]
						if isNearMidpoint(mouseLat, mouseLon, p1, p2, g.zoom) {
							g.vertexEditState.HoveredInsertionID = i
							return
						}
					}
				}
			}
		})
	}

	// First check if we should maintain focus on current line
	if currentLine, ok := g.vertexEditState.lastFocusedObject.(*LineString); ok {
		found := false
		WalkLayers(g.layers[0], func(layer *Layer) {
			if found || !layer.IsEffectivelyVisible() {
				return
			}
			lines := layer.PolylineLayer.Index.Search(currentLine.Bounds())
			for _, l := range lines {
				if l == currentLine && currentLine.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.EditingLine = currentLine
					g.vertexEditState.lastFocusedObject = currentLine

					// Check vertices only for the current focused line
					for i, vertex := range currentLine.Points {
						if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
							g.vertexEditState.HoveredVertexID = i
							found = true
							return
						}
					}
					g.vertexEditState.HoveredVertexID = -1
					found = true
					return
				}
			}
		})

		if found {
			return
		}
		// If we get here, line was not found in any visible layer
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

	// Search through layers in reverse order (top to bottom)
	found := false
	for i := len(g.layers) - 1; i >= 0 && !found; i-- {
		WalkLayers(g.layers[i], func(layer *Layer) {
			if found || !layer.IsEffectivelyVisible() {
				return
			}

			// First check points
			points := layer.PointLayer.Index.Search(searchBounds)
			for _, p := range points {
				point := p.(*Point)
				if point.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.EditingPoint = point
					g.vertexEditState.EditingLine = nil
					g.vertexEditState.EditingPolygon = nil
					g.vertexEditState.HoveredVertexID = 0
					g.vertexEditState.lastFocusedObject = point
					found = true
					return
				}
			}

			// Then check lines
			lines := layer.PolylineLayer.Index.Search(searchBounds)
			for _, l := range lines {
				line := l.(*LineString)
				// First check vertices
				for i, vertex := range line.Points {
					if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
						g.vertexEditState.EditingPoint = nil
						g.vertexEditState.EditingLine = line
						g.vertexEditState.EditingPolygon = nil
						g.vertexEditState.HoveredVertexID = i
						g.vertexEditState.lastFocusedObject = line
						found = true
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
					found = true
					return
				}
			}

			// Finally check polygons
			polygons := layer.PolygonLayer.Index.Search(searchBounds)
			for _, p := range polygons {
				polygon := p.(*Polygon)
				// First check vertices
				for i, vertex := range polygon.Points {
					if vertex.containsPoint(mouseLat, mouseLon, g.zoom) {
						g.vertexEditState.EditingPoint = nil
						g.vertexEditState.EditingLine = nil
						g.vertexEditState.EditingPolygon = polygon
						g.vertexEditState.HoveredVertexID = i
						g.vertexEditState.lastFocusedObject = polygon
						found = true
						return
					}
				}
				// Then check polygon area
				if polygon.containsPoint(mouseLat, mouseLon, g.zoom) {
					g.vertexEditState.EditingPoint = nil
					g.vertexEditState.EditingLine = nil
					g.vertexEditState.EditingPolygon = polygon
					g.vertexEditState.HoveredVertexID = -1
					g.vertexEditState.lastFocusedObject = polygon
					found = true
					return
				}
			}
		})
	}

	// If nothing found, clear edit state
	if !found {
		g.vertexEditState.EditingPoint = nil
		g.vertexEditState.EditingLine = nil
		g.vertexEditState.EditingPolygon = nil
		g.vertexEditState.HoveredVertexID = -1
		g.vertexEditState.lastFocusedObject = nil
	}
}

func (g *Goliath) drawVertexHandles(screen *ebiten.Image) {
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

	// Draw insertion handles for lines
	if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine
		for i := 0; i < len(line.Points)-1; i++ {
			drawInsertionHandle(screen, line.Points[i], line.Points[i+1],
				g.vertexEditState.HoveredInsertionID == i, g)
		}
	}

	// Draw insertion handles for polygons
	if g.vertexEditState.EditingPolygon != nil {
		poly := g.vertexEditState.EditingPolygon
		for i := 0; i < len(poly.Points); i++ {
			p1 := poly.Points[i]
			p2 := poly.Points[(i+1)%len(poly.Points)]
			drawInsertionHandle(screen, p1, p2,
				g.vertexEditState.HoveredInsertionID == i, g)
		}
	}
}

func drawInsertionHandle(screen *ebiten.Image, p1, p2 Point, isHovered bool, g *Goliath) {
	// Calculate midpoint
	midLat := (p1.Lat + p2.Lat) / 2
	midLon := (p1.Lon + p2.Lon) / 2

	// Convert to screen coordinates
	x, y := latLngToPixel(midLat, midLon, g.zoom)
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	screenX := x - (centerX - float64(g.ScreenWidth)/2)
	screenY := y - (centerY - float64(g.ScreenHeight)/2)

	// Draw + symbol
	const size float32 = 5
	handleColor := color.RGBA{128, 128, 128, 255} // Gray
	if isHovered {
		handleColor = color.RGBA{255, 0, 0, 255} // Red when hovered
	}

	// Draw the + symbol
	vector.StrokeLine(screen,
		float32(screenX)-size, float32(screenY),
		float32(screenX)+size, float32(screenY),
		2, handleColor, false)
	vector.StrokeLine(screen,
		float32(screenX), float32(screenY)-size,
		float32(screenX), float32(screenY)+size,
		2, handleColor, false)
}

func (g *Goliath) startVertexDrag() {
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

func (g *Goliath) drawDragPreview(screen *ebiten.Image) {
	if g.vertexEditState == nil {
		return
	}

	// Handle regular vertex drag preview
	if g.vertexEditState.DragState.IsEditing {
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

	// Handle insertion drag preview
	if g.vertexEditState.InsertionDragState.IsEditing {
		mouseX, mouseY := ebiten.CursorPosition()
		centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

		// Draw lines from mouse to segment endpoints
		p1 := g.vertexEditState.InsertionDragState.SegmentStartPoint
		p2 := g.vertexEditState.InsertionDragState.SegmentEndPoint

		x1, y1 := latLngToPixel(p1.Lat, p1.Lon, g.zoom)
		x2, y2 := latLngToPixel(p2.Lat, p2.Lon, g.zoom)
		screenX1 := x1 - (centerX - float64(g.ScreenWidth)/2)
		screenY1 := y1 - (centerY - float64(g.ScreenHeight)/2)
		screenX2 := x2 - (centerX - float64(g.ScreenWidth)/2)
		screenY2 := y2 - (centerY - float64(g.ScreenHeight)/2)

		// Draw preview lines
		vector.StrokeLine(screen,
			float32(mouseX), float32(mouseY),
			float32(screenX1), float32(screenY1),
			1, color.RGBA{255, 0, 0, 255}, false)
		vector.StrokeLine(screen,
			float32(mouseX), float32(mouseY),
			float32(screenX2), float32(screenY2),
			1, color.RGBA{255, 0, 0, 255}, false)
	}
}

func (g *Goliath) finishVertexEdit(mouseX, mouseY int) {
	if g.vertexEditState == nil || !g.vertexEditState.DragState.IsEditing {
		return
	}

	// Find layer containing edited geometry
	var targetLayer *Layer
	for _, rootLayer := range g.layers {
		WalkLayers(rootLayer, func(layer *Layer) {
			if !layer.IsEffectivelyVisible() || targetLayer != nil {
				return
			}

			if g.vertexEditState.EditingPoint != nil {
				points := layer.PointLayer.Index.Search(g.vertexEditState.EditingPoint.Bounds())
				for _, p := range points {
					if p == g.vertexEditState.EditingPoint {
						targetLayer = layer
						return
					}
				}
			} else if g.vertexEditState.EditingLine != nil {
				lines := layer.PolylineLayer.Index.Search(g.vertexEditState.EditingLine.Bounds())
				for _, l := range lines {
					if l == g.vertexEditState.EditingLine {
						targetLayer = layer
						return
					}
				}
			} else if g.vertexEditState.EditingPolygon != nil {
				polygons := layer.PolygonLayer.Index.Search(g.vertexEditState.EditingPolygon.Bounds())
				for _, p := range polygons {
					if p == g.vertexEditState.EditingPolygon {
						targetLayer = layer
						return
					}
				}
			}
		})
		if targetLayer != nil {
			break
		}
	}

	if targetLayer == nil {
		return
	}

	// Calculate new position with snapping
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
		targetLayer.PointLayer.Index.mu.Lock()
		defer targetLayer.PointLayer.Index.mu.Unlock()

		g.clearAffectedTiles(targetLayer, point)
		targetLayer.PointLayer.Index.removeUnsafe(point, point.Bounds())
		point.Lat = newLat
		point.Lon = newLon
		targetLayer.PointLayer.Index.insertUnsafe(point, point.Bounds())
		g.clearAffectedTiles(targetLayer, point)

	} else if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine
		targetLayer.PolylineLayer.Index.mu.Lock()
		defer targetLayer.PolylineLayer.Index.mu.Unlock()

		g.clearAffectedLineTiles(targetLayer, line)
		targetLayer.PolylineLayer.Index.removeUnsafe(line, line.Bounds())
		line.Points[g.vertexEditState.HoveredVertexID].Lat = newLat
		line.Points[g.vertexEditState.HoveredVertexID].Lon = newLon
		targetLayer.PolylineLayer.Index.insertUnsafe(line, line.Bounds())
		g.clearAffectedLineTiles(targetLayer, line)

	} else if g.vertexEditState.EditingPolygon != nil {
		polygon := g.vertexEditState.EditingPolygon
		targetLayer.PolygonLayer.Index.mu.Lock()
		defer targetLayer.PolygonLayer.Index.mu.Unlock()

		g.clearAffectedPolygonTiles(targetLayer, polygon)
		targetLayer.PolygonLayer.Index.removeUnsafe(polygon, polygon.Bounds())
		polygon.Points[g.vertexEditState.HoveredVertexID].Lat = newLat
		polygon.Points[g.vertexEditState.HoveredVertexID].Lon = newLon
		targetLayer.PolygonLayer.Index.insertUnsafe(polygon, polygon.Bounds())
		g.clearAffectedPolygonTiles(targetLayer, polygon)
	}

	// Reset edit state
	g.vertexEditState.DragState.IsEditing = false
	g.vertexEditState.DragState.AdjacentPoints = nil
	g.needRedraw = true
}

func (g *Goliath) insertVertex(mouseX, mouseY int) {
	if g.vertexEditState == nil || g.vertexEditState.HoveredInsertionID < 0 {
		return
	}

	// Find layer containing edited geometry
	var targetLayer *Layer
	for _, rootLayer := range g.layers {
		WalkLayers(rootLayer, func(layer *Layer) {
			if !layer.IsEffectivelyVisible() || targetLayer != nil {
				return
			}

			if g.vertexEditState.EditingLine != nil {
				lines := layer.PolylineLayer.Index.Search(g.vertexEditState.EditingLine.Bounds())
				for _, l := range lines {
					if l == g.vertexEditState.EditingLine {
						targetLayer = layer
						return
					}
				}
			} else if g.vertexEditState.EditingPolygon != nil {
				polygons := layer.PolygonLayer.Index.Search(g.vertexEditState.EditingPolygon.Bounds())
				for _, p := range polygons {
					if p == g.vertexEditState.EditingPolygon {
						targetLayer = layer
						return
					}
				}
			}
		})
		if targetLayer != nil {
			break
		}
	}

	if targetLayer == nil {
		return
	}

	// Calculate insertion position with snapping
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

	// Insert vertex based on geometry type
	if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine
		idx := g.vertexEditState.HoveredInsertionID

		targetLayer.PolylineLayer.Index.mu.Lock()
		defer targetLayer.PolylineLayer.Index.mu.Unlock()

		// Clear tiles for old line
		g.clearAffectedLineTiles(targetLayer, line)

		// Remove from R-tree
		targetLayer.PolylineLayer.Index.removeUnsafe(line, line.Bounds())

		// Insert new point
		newPoints := make([]Point, 0, len(line.Points)+1)
		newPoints = append(newPoints, line.Points[:idx+1]...)
		newPoints = append(newPoints, Point{Lat: newLat, Lon: newLon})
		newPoints = append(newPoints, line.Points[idx+1:]...)
		line.Points = newPoints

		// Re-insert into R-tree
		targetLayer.PolylineLayer.Index.insertUnsafe(line, line.Bounds())

		// Clear tiles for new line
		g.clearAffectedLineTiles(targetLayer, line)

	} else if g.vertexEditState.EditingPolygon != nil {
		polygon := g.vertexEditState.EditingPolygon
		idx := g.vertexEditState.HoveredInsertionID

		targetLayer.PolygonLayer.Index.mu.Lock()
		defer targetLayer.PolygonLayer.Index.mu.Unlock()

		// Clear tiles for old polygon
		g.clearAffectedPolygonTiles(targetLayer, polygon)

		// Remove from R-tree
		targetLayer.PolygonLayer.Index.removeUnsafe(polygon, polygon.Bounds())

		// Insert new point
		newPoints := make([]Point, 0, len(polygon.Points)+1)
		newPoints = append(newPoints, polygon.Points[:idx+1]...)
		newPoints = append(newPoints, Point{Lat: newLat, Lon: newLon})
		newPoints = append(newPoints, polygon.Points[idx+1:]...)
		polygon.Points = newPoints

		// Re-insert into R-tree
		targetLayer.PolygonLayer.Index.insertUnsafe(polygon, polygon.Bounds())

		// Clear tiles for new polygon
		g.clearAffectedPolygonTiles(targetLayer, polygon)
	}

	g.needRedraw = true
}

func (g *Goliath) startInsertionDrag() {
	if g.vertexEditState == nil || g.vertexEditState.HoveredInsertionID < 0 {
		return
	}

	// Get segment endpoints based on geometry type
	var p1, p2 Point
	if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine
		idx := g.vertexEditState.HoveredInsertionID
		p1 = line.Points[idx]
		p2 = line.Points[idx+1]
	} else if g.vertexEditState.EditingPolygon != nil {
		poly := g.vertexEditState.EditingPolygon
		idx := g.vertexEditState.HoveredInsertionID
		p1 = poly.Points[idx]
		p2 = poly.Points[(idx+1)%len(poly.Points)]
	}

	g.vertexEditState.InsertionDragState = struct {
		IsEditing         bool
		SegmentStartPoint Point
		SegmentEndPoint   Point
	}{
		IsEditing:         true,
		SegmentStartPoint: p1,
		SegmentEndPoint:   p2,
	}
}

func (g *Goliath) finishInsertionDrag(mouseX, mouseY int) {
	if g.vertexEditState == nil || !g.vertexEditState.InsertionDragState.IsEditing {
		return
	}

	g.insertVertex(mouseX, mouseY)
	g.vertexEditState.InsertionDragState.IsEditing = false
	g.needRedraw = true
}

func (g *Goliath) deleteVertex() {
	if g.vertexEditState == nil || g.vertexEditState.HoveredVertexID < 0 {
		return
	}

	// Find layer containing edited geometry
	var targetLayer *Layer
	for _, layer := range g.layers {
		if !layer.Visible {
			continue
		}

		if g.vertexEditState.EditingLine != nil {
			lines := layer.PolylineLayer.Index.Search(g.vertexEditState.EditingLine.Bounds())
			for _, l := range lines {
				if l == g.vertexEditState.EditingLine {
					targetLayer = layer
					break
				}
			}
		} else if g.vertexEditState.EditingPolygon != nil {
			polygons := layer.PolygonLayer.Index.Search(g.vertexEditState.EditingPolygon.Bounds())
			for _, p := range polygons {
				if p == g.vertexEditState.EditingPolygon {
					targetLayer = layer
					break
				}
			}
		}

		if targetLayer != nil {
			break
		}
	}

	if targetLayer == nil {
		return
	}

	if g.vertexEditState.EditingLine != nil {
		line := g.vertexEditState.EditingLine

		// Don't delete if it would make the line too short
		if len(line.Points) <= 2 {
			return
		}

		targetLayer.PolylineLayer.Index.mu.Lock()
		defer targetLayer.PolylineLayer.Index.mu.Unlock()

		// Clear tiles for old line
		g.clearAffectedLineTiles(targetLayer, line)

		// Remove from R-tree
		targetLayer.PolylineLayer.Index.removeUnsafe(line, line.Bounds())

		// Remove vertex
		newPoints := make([]Point, 0, len(line.Points)-1)
		newPoints = append(newPoints, line.Points[:g.vertexEditState.HoveredVertexID]...)
		newPoints = append(newPoints, line.Points[g.vertexEditState.HoveredVertexID+1:]...)
		line.Points = newPoints

		// Re-insert into R-tree
		targetLayer.PolylineLayer.Index.insertUnsafe(line, line.Bounds())
		g.clearAffectedLineTiles(targetLayer, line)

	} else if g.vertexEditState.EditingPolygon != nil {
		polygon := g.vertexEditState.EditingPolygon

		// Don't delete if it would make the polygon too small
		if len(polygon.Points) <= 3 {
			return
		}

		targetLayer.PolygonLayer.Index.mu.Lock()
		defer targetLayer.PolygonLayer.Index.mu.Unlock()

		// Clear tiles for old polygon
		g.clearAffectedPolygonTiles(targetLayer, polygon)

		// Remove from R-tree
		targetLayer.PolygonLayer.Index.removeUnsafe(polygon, polygon.Bounds())

		// Remove vertex
		newPoints := make([]Point, 0, len(polygon.Points)-1)
		newPoints = append(newPoints, polygon.Points[:g.vertexEditState.HoveredVertexID]...)
		newPoints = append(newPoints, polygon.Points[g.vertexEditState.HoveredVertexID+1:]...)
		polygon.Points = newPoints

		// Re-insert into R-tree
		targetLayer.PolygonLayer.Index.insertUnsafe(polygon, polygon.Bounds())
		g.clearAffectedPolygonTiles(targetLayer, polygon)
	}

	// Reset vertex edit state
	g.vertexEditState = nil
	g.needRedraw = true
}
