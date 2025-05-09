package tilemap

import (
	"math"

	"github.com/OpticalFlyer/goliath/proj"
)

// PanDirection represents a direction to pan the map
type PanDirection int

const (
	PanLeft PanDirection = iota
	PanRight
	PanUp
	PanDown
)

// PanSpeed in pixels per frame
const PanSpeed = 50

// Pan moves the map center in the specified direction by a fixed number of pixels
func (tm *TileMap) Pan(dir PanDirection) {
	switch dir {
	case PanLeft:
		tm.PanBy(PanSpeed, 0)
	case PanRight:
		tm.PanBy(-PanSpeed, 0)
	case PanUp:
		tm.PanBy(0, PanSpeed)
	case PanDown:
		tm.PanBy(0, -PanSpeed)
	}
}

// PanBy moves the map by pixel offsets
// dx,dy are in screen pixels, positive dx moves map west (view east), positive dy moves map south (view north)
func (tm *TileMap) PanBy(dx, dy float64) {
	// Convert pixel offsets to tile coordinates at current zoom level
	pixelsToTiles := 1.0 / TileSize
	tileDX := dx * pixelsToTiles
	tileDY := dy * pixelsToTiles

	// Get current center in tile coordinates
	centerTileX, centerTileY := proj.LatLonToTileCoords(tm.CenterLat, tm.CenterLon, tm.Zoom)

	// Move in tile space
	newCenterTileX := centerTileX - tileDX
	newCenterTileY := centerTileY - tileDY

	// Get max tile coordinate for current zoom
	maxTileCoord := float64(uint(1) << uint(tm.Zoom))

	// Clamp X and Y to valid tile ranges (no wrapping)
	newCenterTileX = math.Max(0, math.Min(maxTileCoord, newCenterTileX))
	newCenterTileY = math.Max(0, math.Min(maxTileCoord, newCenterTileY))

	// Convert tile coordinates back to lat/lon
	lon := (newCenterTileX/maxTileCoord)*360.0 - 180.0
	lat := math.Atan(math.Sinh(math.Pi*(1-2*newCenterTileY/maxTileCoord))) * 180.0 / math.Pi

	tm.CenterLon = lon
	tm.CenterLat = lat
}
