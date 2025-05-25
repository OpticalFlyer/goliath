package proj

import "math"

// Constants for Web Mercator projection
const (
	maxLat   = 85.0511 // Maximum latitude in Web Mercator (arctan(sinh(π)))
	minLat   = -85.0511
	degToRad = math.Pi / 180.0
	radToDeg = 180.0 / math.Pi
)

// pow2 contains pre-calculated powers of 2 for zoom levels 0-21
var pow2 = [22]float64{
	1, 2, 4, 8, 16, 32, 64, 128, 256, 512,
	1024, 2048, 4096, 8192, 16384, 32768, 65536,
	131072, 262144, 524288, 1048576, 2097152,
}

// LatLonToTileCoords converts WGS84 coordinates to Web Mercator tile coordinates
// at the specified zoom level.
//
// Parameters:
//   - lat: Latitude in degrees (clamped to -85.0511 to 85.0511)
//   - lon: Longitude in degrees (-180 to 180)
//   - zoom: Zoom level (0-19)
//
// Returns:
//   - x: Tile X coordinate (fractional)
//   - y: Tile Y coordinate (fractional)
func LatLonToTileCoords(lat, lon float64, zoom int) (x, y float64) {
	// Clamp latitude using direct comparison
	if lat > maxLat {
		lat = maxLat
	} else if lat < minLat {
		lat = minLat
	}

	// Get tile scale using pre-calculated powers
	n := pow2[zoom]

	// Calculate x coordinate (simplified)
	x = (lon + 180.0) * (n / 360.0)

	// Handle y coordinate edge cases first
	if lat >= maxLat {
		return x, 0
	}
	if lat <= minLat {
		return x, n
	}

	// Calculate y coordinate using optimized formula
	latRad := lat * degToRad
	sinLat := math.Sin(latRad)
	y = n * (0.5 - 0.25*math.Log((1.0+sinLat)/(1.0-sinLat))/math.Pi)

	return x, y
}

// WebMercatorToTileCoords converts Web Mercator (EPSG:3857) coordinates in meters
// to tile coordinates at the specified zoom level.
//
// Parameters:
//   - x: X coordinate in meters (-20037508.34 to 20037508.34)
//   - y: Y coordinate in meters (-20037508.34 to 20037508.34)
//   - zoom: Zoom level (0-19)
//
// Returns:
//   - tileX: Tile X coordinate (fractional)
//   - tileY: Tile Y coordinate (fractional)
func WebMercatorToTileCoords(x, y float64, zoom int) (tileX, tileY float64) {
	// Web Mercator bounds in meters
	const maxMeters = 20037508.34

	// Normalize coordinates to 0-1 range
	normalizedX := (x + maxMeters) / (2 * maxMeters)
	normalizedY := 1 - ((y + maxMeters) / (2 * maxMeters))

	// Scale to tile coordinates
	n := pow2[zoom]
	tileX = normalizedX * n
	tileY = normalizedY * n

	return tileX, tileY
}

// WebMercatorToScreenCoords converts Web Mercator (EPSG:3857) coordinates in meters
// to screen pixel coordinates relative to the top-left of the visible map area.
//
// Parameters:
//   - x: X coordinate in meters.
//   - y: Y coordinate in meters.
//   - zoom: The current map zoom level.
//   - mapTopLeftPixelX: The X coordinate (in world pixels) of the map's
//     top-left corner currently visible on the screen.
//   - mapTopLeftPixelY: The Y coordinate (in world pixels) of the map's
//     top-left corner currently visible on the screen.
//   - tileSize: The display size of a single map tile in pixels (e.g., 256).
//
// Returns:
//   - screenX: The X coordinate on the screen in pixels.
//   - screenY: The Y coordinate on the screen in pixels.
func WebMercatorToScreenCoords(x, y float64, zoom int, mapTopLeftPixelX, mapTopLeftPixelY, tileSize float64) (screenX, screenY float64) {
	// Convert Web Mercator to tile coordinates
	tileX, tileY := WebMercatorToTileCoords(x, y, zoom)

	// Convert tile coordinates to world pixel coordinates
	worldPixelX := tileX * tileSize
	worldPixelY := tileY * tileSize

	// Convert world pixel coordinates to screen coordinates
	screenX = worldPixelX - mapTopLeftPixelX
	screenY = worldPixelY - mapTopLeftPixelY

	return screenX, screenY
}
