// geometry.go
package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type Point struct {
	Lat       float64
	Lon       float64
	Selected  bool
	IconImage *ebiten.Image
	Scale     float64
	HotSpot   HotSpot
}

type LineString struct {
	Points   []Point
	Selected bool
	Color    color.RGBA
	Width    float32
}

type Polygon struct {
	Points   []Point
	Selected bool
}

func NewPoint(lat, lon float64) *Point {
	return &Point{
		Lat: lat,
		Lon: lon,
	}
}

// Calculate bounds for R-tree indexing
func (p *Point) Bounds() Bounds {
	return Bounds{
		MinX: p.Lon - 0.0001, // Small buffer in degrees
		MinY: p.Lat - 0.0001,
		MaxX: p.Lon + 0.0001,
		MaxY: p.Lat + 0.0001,
	}
}

func (l *LineString) Bounds() Bounds {
	if len(l.Points) == 0 {
		return Bounds{}
	}
	bounds := Bounds{
		MinX: l.Points[0].Lon,
		MinY: l.Points[0].Lat,
		MaxX: l.Points[0].Lon,
		MaxY: l.Points[0].Lat,
	}
	for _, p := range l.Points[1:] {
		bounds.MinX = math.Min(bounds.MinX, p.Lon)
		bounds.MinY = math.Min(bounds.MinY, p.Lat)
		bounds.MaxX = math.Max(bounds.MaxX, p.Lon)
		bounds.MaxY = math.Max(bounds.MaxY, p.Lat)
	}
	return bounds
}

func (p *Polygon) Bounds() Bounds {
	minX, maxX := p.Points[0].Lon, p.Points[0].Lon
	minY, maxY := p.Points[0].Lat, p.Points[0].Lat
	for _, pt := range p.Points[1:] {
		if pt.Lon < minX {
			minX = pt.Lon
		}
		if pt.Lon > maxX {
			maxX = pt.Lon
		}
		if pt.Lat < minY {
			minY = pt.Lat
		}
		if pt.Lat > maxY {
			maxY = pt.Lat
		}
	}
	return Bounds{
		MinX: minX,
		MinY: minY,
		MaxX: maxX,
		MaxY: maxY,
	}
}

func WalkLayers(layer *Layer, fn func(*Layer)) {
	fn(layer)
	for _, child := range layer.Children {
		WalkLayers(child, fn)
	}
}

func (p *Point) containsPoint(lat, lon float64, zoom int) bool {
	// Use pixel-based selection (6 pixel radius)
	const pixelRadius = 6.0

	// Convert point and click locations to pixels
	px1, py1 := latLngToPixel(p.Lat, p.Lon, zoom)
	px2, py2 := latLngToPixel(lat, lon, zoom)

	// Calculate distance in pixels
	dx := px2 - px1
	dy := py2 - py1
	distanceInPixels := math.Sqrt(dx*dx + dy*dy)

	return distanceInPixels <= pixelRadius
}

func (l *LineString) containsPoint(lat, lon float64, zoom int) bool {
	// Use pixel-based selection (3 pixel distance from line)
	const pixelThreshold = 3.0

	clickX, clickY := latLngToPixel(lat, lon, zoom)

	for i := 0; i < len(l.Points)-1; i++ {
		p1, p2 := l.Points[i], l.Points[i+1]
		x1, y1 := latLngToPixel(p1.Lat, p1.Lon, zoom)
		x2, y2 := latLngToPixel(p2.Lat, p2.Lon, zoom)

		// Calculate distance from point to line segment in pixels
		A := clickX - x1
		B := clickY - y1
		C := x2 - x1
		D := y2 - y1

		dot := A*C + B*D
		lenSq := C*C + D*D

		if lenSq == 0 {
			continue
		}

		param := dot / lenSq
		var x, y float64

		if param < 0 {
			x, y = x1, y1
		} else if param > 1 {
			x, y = x2, y2
		} else {
			x = x1 + param*C
			y = y1 + param*D
		}

		distInPixels := math.Sqrt((clickX-x)*(clickX-x) + (clickY-y)*(clickY-y))
		if distInPixels <= pixelThreshold {
			return true
		}
	}
	return false
}

func (p *Polygon) containsPoint(lat, lon float64, zoom int) bool {
	// First check edges with pixel-based buffer
	const pixelThreshold = 3.0 // Same as line threshold
	clickX, clickY := latLngToPixel(lat, lon, zoom)

	// Check if point is near any edge using pixel distance
	j := len(p.Points) - 1
	for i := 0; i < len(p.Points); i++ {
		p1, p2 := p.Points[j], p.Points[i]
		x1, y1 := latLngToPixel(p1.Lat, p1.Lon, zoom)
		x2, y2 := latLngToPixel(p2.Lat, p2.Lon, zoom)

		// Calculate distance from point to line segment in pixels
		A := clickX - x1
		B := clickY - y1
		C := x2 - x1
		D := y2 - y1

		dot := A*C + B*D
		lenSq := C*C + D*D

		if lenSq != 0 {
			param := dot / lenSq
			var x, y float64

			if param < 0 {
				x, y = x1, y1
			} else if param > 1 {
				x, y = x2, y2
			} else {
				x = x1 + param*C
				y = y1 + param*D
			}

			distInPixels := math.Sqrt((clickX-x)*(clickX-x) + (clickY-y)*(clickY-y))
			if distInPixels <= pixelThreshold {
				return true
			}
		}
		j = i
	}

	// If not on edge, check if inside polygon using ray casting
	inside := false
	j = len(p.Points) - 1
	for i := 0; i < len(p.Points); i++ {
		if ((p.Points[i].Lat > lat) != (p.Points[j].Lat > lat)) &&
			(lon < (p.Points[j].Lon-p.Points[i].Lon)*(lat-p.Points[i].Lat)/
				(p.Points[j].Lat-p.Points[i].Lat)+p.Points[i].Lon) {
			inside = !inside
		}
		j = i
	}

	return inside
}

func (l *LineString) intersectsBox(bounds Bounds) bool {
	// Check if any line segment intersects with the box
	for i := 0; i < len(l.Points)-1; i++ {
		p1, p2 := l.Points[i], l.Points[i+1]

		// Line segment coordinates
		x1, y1 := p1.Lon, p1.Lat
		x2, y2 := p2.Lon, p2.Lat

		// Box coordinates
		left := bounds.MinX
		right := bounds.MaxX
		top := bounds.MaxY
		bottom := bounds.MinY

		// Check if line segment intersects with any of the box edges
		// Using Cohen-Sutherland line clipping algorithm
		outcode1 := computeOutCode(x1, y1, left, right, top, bottom)
		outcode2 := computeOutCode(x2, y2, left, right, top, bottom)

		for {
			if (outcode1 | outcode2) == 0 {
				// Line segment is completely inside the box
				return true
			}
			if (outcode1 & outcode2) != 0 {
				// Line segment is completely outside the box
				break
			}

			// Line segment crosses the box - need to check intersection
			x, y := 0.0, 0.0
			outcodeOut := outcode1
			if outcodeOut == 0 {
				outcodeOut = outcode2
			}

			// Calculate intersection point
			if (outcodeOut & 8) != 0 { // Above
				x = x1 + (x2-x1)*(top-y1)/(y2-y1)
				y = top
			} else if (outcodeOut & 4) != 0 { // Below
				x = x1 + (x2-x1)*(bottom-y1)/(y2-y1)
				y = bottom
			} else if (outcodeOut & 2) != 0 { // Right
				y = y1 + (y2-y1)*(right-x1)/(x2-x1)
				x = right
			} else if (outcodeOut & 1) != 0 { // Left
				y = y1 + (y2-y1)*(left-x1)/(x2-x1)
				x = left
			}

			if outcodeOut == outcode1 {
				x1, y1 = x, y
				outcode1 = computeOutCode(x1, y1, left, right, top, bottom)
			} else {
				x2, y2 = x, y
				outcode2 = computeOutCode(x2, y2, left, right, top, bottom)
			}
		}
	}
	return false
}

func computeOutCode(x, y, left, right, top, bottom float64) int {
	code := 0
	if y > top {
		code |= 8 // Above
	} else if y < bottom {
		code |= 4 // Below
	}
	if x > right {
		code |= 2 // Right
	} else if x < left {
		code |= 1 // Left
	}
	return code
}

func (p *Polygon) intersectsBox(bounds Bounds) bool {
	// 1. Check if any polygon vertex is inside the box
	for _, pt := range p.Points {
		if pt.Lon >= bounds.MinX && pt.Lon <= bounds.MaxX &&
			pt.Lat >= bounds.MinY && pt.Lat <= bounds.MaxY {
			return true
		}
	}

	// 2. Check if any polygon edge intersects with the box edges
	boxVertices := [][2]float64{
		{bounds.MinX, bounds.MinY}, // Bottom-left
		{bounds.MaxX, bounds.MinY}, // Bottom-right
		{bounds.MaxX, bounds.MaxY}, // Top-right
		{bounds.MinX, bounds.MaxY}, // Top-left
	}

	// Check polygon edges against box edges
	j := len(p.Points) - 1
	for i := 0; i < len(p.Points); i++ {
		p1, p2 := p.Points[j], p.Points[i]

		// Check if this polygon edge intersects any box edge
		for k := 0; k < 4; k++ {
			b1, b2 := boxVertices[k], boxVertices[(k+1)%4]

			// Line segment intersection test
			if linesIntersect(
				p1.Lon, p1.Lat, p2.Lon, p2.Lat,
				b1[0], b1[1], b2[0], b2[1]) {
				return true
			}
		}
		j = i
	}

	// 3. Check if box is completely inside polygon
	// Check all four corners of the box
	corners := [][2]float64{
		{bounds.MinX, bounds.MinY},
		{bounds.MaxX, bounds.MinY},
		{bounds.MaxX, bounds.MaxY},
		{bounds.MinX, bounds.MaxY},
	}

	// If any corner is outside, the box isn't contained
	allCornersInside := true
	for _, corner := range corners {
		if !pointInPolygon(corner[0], corner[1], p.Points) {
			allCornersInside = false
			break
		}
	}

	return allCornersInside
}

// Add these helper functions:
func linesIntersect(x1, y1, x2, y2, x3, y3, x4, y4 float64) bool {
	// Calculate denominators for parameters
	denom := (y4-y3)*(x2-x1) - (x4-x3)*(y2-y1)
	if denom == 0 {
		return false // Lines are parallel
	}

	ua := ((x4-x3)*(y1-y3) - (y4-y3)*(x1-x3)) / denom
	ub := ((x2-x1)*(y1-y3) - (y2-y1)*(x1-x3)) / denom

	// Return true if the intersection lies within both line segments
	return ua >= 0 && ua <= 1 && ub >= 0 && ub <= 1
}

func pointInPolygon(x, y float64, points []Point) bool {
	inside := false
	j := len(points) - 1

	for i := 0; i < len(points); i++ {
		if ((points[i].Lat > y) != (points[j].Lat > y)) &&
			(x < (points[j].Lon-points[i].Lon)*(y-points[i].Lat)/
				(points[j].Lat-points[i].Lat)+points[i].Lon) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// haversineDistance calculates the great-circle distance between two points
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	// Difference in coordinates
	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	// Haversine formula
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	// Earth's radius in feet (3959 miles * 5280 feet/mile)
	earthRadiusFeet := 20894400.0

	return earthRadiusFeet * c
}

// roundToNearestFoot rounds a float64 distance to nearest foot
func roundToNearestFoot(dist float64) int {
	return int(math.Round(dist))
}
