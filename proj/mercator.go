package proj

import "math"

// Constants for Web Mercator projection
const (
	maxLat   = 85.0511 // Maximum latitude in Web Mercator (arctan(sinh(π)))
	minLat   = -85.0511
	degToRad = math.Pi / 180.0
	radToDeg = 180.0 / math.Pi
)

// pow2 contains pre-calculated powers of 2 for zoom levels 0-19
var pow2 = [20]float64{
	1, 2, 4, 8, 16, 32, 64, 128, 256, 512,
	1024, 2048, 4096, 8192, 16384, 32768, 65536,
	131072, 262144, 524288,
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
