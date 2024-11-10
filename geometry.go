// geometry.go
package main

import (
	"math"
)

type Point struct {
	Lat float64 // WGS84 latitude
	Lon float64 // WGS84 longitude
}

// LineString represents a polyline as a series of points
type LineString struct {
	Points []Point
}

// Polygon represents a polygon as a series of points
// First ring is exterior, subsequent rings are holes
type Polygon struct {
	Rings [][]Point
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
	if len(p.Rings) == 0 || len(p.Rings[0]) == 0 {
		return Bounds{}
	}
	bounds := Bounds{
		MinX: p.Rings[0][0].Lon,
		MinY: p.Rings[0][0].Lat,
		MaxX: p.Rings[0][0].Lon,
		MaxY: p.Rings[0][0].Lat,
	}
	for _, ring := range p.Rings {
		for _, p := range ring {
			bounds.MinX = math.Min(bounds.MinX, p.Lon)
			bounds.MinY = math.Min(bounds.MinY, p.Lat)
			bounds.MaxX = math.Max(bounds.MaxX, p.Lon)
			bounds.MaxY = math.Max(bounds.MaxY, p.Lat)
		}
	}
	return bounds
}
