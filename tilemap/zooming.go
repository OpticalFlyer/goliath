package tilemap

import (
	"math"
)

// ZoomIn increases the zoom level if not at max zoom
func (tm *TileMap) ZoomIn() {
	if tm.Zoom < MaxZoomLevel {
		tm.Zoom++
	}
}

// ZoomOut decreases the zoom level if not at minimum zoom
func (tm *TileMap) ZoomOut() {
	if tm.Zoom > 0 {
		tm.Zoom--
	}
}

// ScreenToWorld converts screen coordinates to tile coordinates
func (tm *TileMap) ScreenToWorld(screenX, screenY float64) (tileX, tileY float64) {
	// Get current center in tile coordinates
	centerTileX, centerTileY := LatLonToTileFloat(tm.CenterLat, tm.CenterLon, tm.Zoom)

	// Convert screen coords to tile coords relative to center
	pixelsToTiles := 1.0 / TileSize
	tileX = centerTileX + (screenX-float64(tm.ScreenWidth)/2)*pixelsToTiles
	tileY = centerTileY + (screenY-float64(tm.ScreenHeight)/2)*pixelsToTiles

	return tileX, tileY
}

// ZoomAtPoint zooms the map while keeping the given world point at the same screen location
func (tm *TileMap) ZoomAtPoint(zoomIn bool, screenX, screenY float64) {
	if (zoomIn && tm.Zoom >= MaxZoomLevel) || (!zoomIn && tm.Zoom <= 0) {
		return
	}

	// Get mouse position in tile coordinates before zoom
	mouseWorldX, mouseWorldY := tm.ScreenToWorld(screenX, screenY)

	// Check if mouse is within world bounds
	maxTileCoord := float64(uint(1) << uint(tm.Zoom))
	if mouseWorldX < 0 || mouseWorldX > maxTileCoord ||
		mouseWorldY < 0 || mouseWorldY > maxTileCoord {
		return // Don't zoom if cursor is outside world bounds
	}

	// Change zoom level
	oldZoom := tm.Zoom
	if zoomIn {
		tm.Zoom++
	} else {
		tm.Zoom--
	}

	// Calculate the scale factor between zoom levels
	scaleFactor := math.Pow(2, float64(tm.Zoom-oldZoom))

	// Convert mouse world position to the new zoom level
	mouseWorldXNewZoom := mouseWorldX * scaleFactor
	mouseWorldYNewZoom := mouseWorldY * scaleFactor

	// Convert screen position to tile offset at new zoom level
	pixelsToTiles := 1.0 / TileSize
	screenTileOffsetX := (screenX - float64(tm.ScreenWidth)/2) * pixelsToTiles
	screenTileOffsetY := (screenY - float64(tm.ScreenHeight)/2) * pixelsToTiles

	// Calculate new center in tile coordinates
	newCenterTileX := mouseWorldXNewZoom - screenTileOffsetX
	newCenterTileY := mouseWorldYNewZoom - screenTileOffsetY

	// Convert back to lat/lon
	maxTileCoord = float64(uint(1) << uint(tm.Zoom))
	lon := (newCenterTileX/maxTileCoord)*360.0 - 180.0
	lat := math.Atan(math.Sinh(math.Pi*(1-2*newCenterTileY/maxTileCoord))) * 180.0 / math.Pi

	// Clamp to valid ranges
	tm.CenterLon = math.Max(-180.0, math.Min(180.0, lon))
	tm.CenterLat = math.Max(-85.0511, math.Min(85.0511, lat))
}
