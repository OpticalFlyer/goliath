// geometry.go
package main

import (
	"math"
)

type Point struct {
	Lat      float64
	Lon      float64
	Selected bool
}

type LineString struct {
	Points   []Point
	Selected bool
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

func (p *Point) containsPoint(lat, lon float64, zoom int) bool {
	// Use pixel-based selection (5 pixel radius)
	const pixelRadius = 5.0

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
	// First check edges with buffer based on zoom level
	baseBuffer := 0.0001 // Base threshold in degrees
	zoomFactor := math.Pow(0.7, float64(zoom-5))
	threshold := baseBuffer * zoomFactor

	// Check if point is near any edge
	j := len(p.Points) - 1
	for i := 0; i < len(p.Points); i++ {
		p1, p2 := p.Points[j], p.Points[i]

		// Calculate distance from point to line segment
		A := lon - p1.Lon
		B := lat - p1.Lat
		C := p2.Lon - p1.Lon
		D := p2.Lat - p1.Lat

		dot := A*C + B*D
		lenSq := C*C + D*D

		if lenSq != 0 {
			param := dot / lenSq
			var x, y float64

			if param < 0 {
				x, y = p1.Lon, p1.Lat
			} else if param > 1 {
				x, y = p2.Lon, p2.Lat
			} else {
				x = p1.Lon + param*C
				y = p1.Lat + param*D
			}

			dist := math.Sqrt((lon-x)*(lon-x) + (lat-y)*(lat-y))
			if dist <= threshold {
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
