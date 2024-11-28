package main

import (
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Layer represents a collection of geometry types
type Layer struct {
	Name             string
	Visible          bool
	Parent           *Layer   // Reference to parent layer
	Children         []*Layer // Child layers
	PointLayer       *GeometryLayer
	PointTileCache   *PointTileCache
	PolylineLayer    *GeometryLayer
	LineTileCache    *LineTileCache
	PolygonLayer     *GeometryLayer
	PolygonTileCache *PolygonTileCache
	Expanded         bool // UI state for layer panel
	cachedBounds     *Bounds
	cachedBoundsMu   sync.RWMutex
}

func computeLayerGeometryBounds(layer *Layer) Bounds {
	bounds := Bounds{
		MinX: math.MaxFloat64,
		MinY: math.MaxFloat64,
		MaxX: -math.MaxFloat64,
		MaxY: -math.MaxFloat64,
	}

	// Points
	points := layer.PointLayer.Index.Search(Bounds{MinX: -180, MinY: -90, MaxX: 180, MaxY: 90})
	for _, p := range points {
		point := p.(*Point)
		bounds = expandBounds(bounds, point.Bounds())
	}

	// Lines
	lines := layer.PolylineLayer.Index.Search(Bounds{MinX: -180, MinY: -90, MaxX: 180, MaxY: 90})
	for _, l := range lines {
		line := l.(*LineString)
		bounds = expandBounds(bounds, line.Bounds())
	}

	// Polygons
	polygons := layer.PolygonLayer.Index.Search(Bounds{MinX: -180, MinY: -90, MaxX: 180, MaxY: 90})
	for _, p := range polygons {
		polygon := p.(*Polygon)
		bounds = expandBounds(bounds, polygon.Bounds())
	}

	// Include child layer bounds
	for _, child := range layer.Children {
		childBounds := child.GetBounds()
		bounds = expandBounds(bounds, childBounds)
	}

	return bounds
}

// Update Layer struct to track bounds validity
func (l *Layer) invalidateBounds() {
	l.cachedBoundsMu.Lock()
	defer l.cachedBoundsMu.Unlock()
	l.cachedBounds = nil
}

// Update GetBounds to handle empty layers
func (l *Layer) GetBounds() Bounds {
	l.cachedBoundsMu.RLock()
	if l.cachedBounds != nil {
		bounds := *l.cachedBounds
		l.cachedBoundsMu.RUnlock()
		return bounds
	}
	l.cachedBoundsMu.RUnlock()

	l.cachedBoundsMu.Lock()
	defer l.cachedBoundsMu.Unlock()

	bounds := computeLayerGeometryBounds(l)

	// Handle empty layer case
	if bounds.MinX == math.MaxFloat64 {
		bounds = Bounds{
			MinX: -180,
			MinY: -90,
			MaxX: 180,
			MaxY: 90,
		}
	}

	l.cachedBounds = &bounds
	return bounds
}

func expandBounds(a, b Bounds) Bounds {
	return Bounds{
		MinX: math.Min(a.MinX, b.MinX),
		MinY: math.Min(a.MinY, b.MinY),
		MaxX: math.Max(a.MaxX, b.MaxX),
		MaxY: math.Max(a.MaxY, b.MaxY),
	}
}

// Check if layer is effectively visible (considering parent visibility)
func (l *Layer) IsEffectivelyVisible() bool {
	current := l
	for current != nil {
		if !current.Visible {
			return false
		}
		current = current.Parent
	}
	return true
}

// Add child layer
func (l *Layer) AddChild(child *Layer) {
	child.Parent = l
	l.Children = append(l.Children, child)
	l.invalidateBounds()

	// Invalidate parent bounds up the tree
	current := l.Parent
	for current != nil {
		current.invalidateBounds()
		current = current.Parent
	}
}

// NewLayer creates a new layer with initialized geometry layers
func NewLayer(name string, screenWidth, screenHeight int) *Layer {
	layer := &Layer{
		Name:     name,
		Visible:  true,
		Children: make([]*Layer, 0),
		Expanded: true,
		PointLayer: &GeometryLayer{
			Index:  NewRTree(),
			buffer: ebiten.NewImage(screenWidth, screenHeight),
		},
		PolylineLayer: &GeometryLayer{
			Index: NewRTree(),
		},
		PolygonLayer: &GeometryLayer{
			Index: NewRTree(),
		},
		PointTileCache:   NewPointTileCache(1000),
		LineTileCache:    NewLineTileCache(1000),
		PolygonTileCache: NewPolygonTileCache(1000),
	}

	// Initialize cache maps
	layer.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	layer.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
	layer.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)

	return layer
}
