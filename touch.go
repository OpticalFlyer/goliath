package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Goliath) handleTouchEvents() {
	// Use AppendTouchIDs instead of TouchIDs
	touches := make([]ebiten.TouchID, 0, 8)
	touches = ebiten.AppendTouchIDs(touches)

	// Initialize touch tracking maps if needed
	if g.lastTouchX == nil {
		g.lastTouchX = make(map[ebiten.TouchID]float64)
		g.lastTouchY = make(map[ebiten.TouchID]float64)
	}

	// Handle touch start
	for _, id := range touches {
		if _, exists := g.lastTouchX[id]; !exists {
			x, y := ebiten.TouchPosition(id)
			g.lastTouchX[id] = float64(x)
			g.lastTouchY[id] = float64(y)
		}
	}

	// Clean up ended touches
	for id := range g.lastTouchX {
		if !containsTouchID(touches, id) {
			delete(g.lastTouchX, id)
			delete(g.lastTouchY, id)
		}
	}

	switch len(touches) {
	case 1: // Single touch - pan
		id := touches[0]
		x, y := ebiten.TouchPosition(id)
		if lastX, ok := g.lastTouchX[id]; ok {
			if lastY, ok := g.lastTouchY[id]; ok {
				dx := float64(x) - lastX
				dy := float64(y) - lastY
				if dx != 0 || dy != 0 {
					g.tileMap.PanBy(dx, dy)
				}
			}
		}
		g.lastTouchX[id] = float64(x)
		g.lastTouchY[id] = float64(y)

	case 2: // Two finger touch - pinch to zoom
		id1, id2 := touches[0], touches[1]
		x1, y1 := ebiten.TouchPosition(id1)
		x2, y2 := ebiten.TouchPosition(id2)

		currentDist := distance(float64(x1), float64(y1), float64(x2), float64(y2))

		if _, ok := g.lastTouchX[id1]; ok {
			if _, ok := g.lastTouchX[id2]; ok {
				prevDist := distance(g.lastTouchX[id1], g.lastTouchY[id1],
					g.lastTouchX[id2], g.lastTouchY[id2])

				midX := (float64(x1) + float64(x2)) / 2
				midY := (float64(y1) + float64(y2)) / 2

				if currentDist > prevDist*1.1 { // Zoom in
					g.tileMap.ZoomAtPoint(true, midX, midY)
				} else if currentDist < prevDist*0.9 { // Zoom out
					g.tileMap.ZoomAtPoint(false, midX, midY)
				}
			}
		}

		g.lastTouchX[id1], g.lastTouchY[id1] = float64(x1), float64(y1)
		g.lastTouchX[id2], g.lastTouchY[id2] = float64(x2), float64(y2)
	}
}

// Helper function to check if a TouchID is in a slice
func containsTouchID(ids []ebiten.TouchID, id ebiten.TouchID) bool {
	for _, tid := range ids {
		if tid == id {
			return true
		}
	}
	return false
}

// Helper function to calculate distance between two points
func distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}
