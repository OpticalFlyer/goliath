// polygon.go
package main

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

var whiteImage = ebiten.NewImage(3, 3)

func init() {
	whiteImage.Fill(color.White)
}

// PolygonTile represents a cached rendering of polygons for a specific tile
type PolygonTile struct {
	Image     *ebiten.Image
	Bounds    Bounds
	ZoomLevel int
}

// PolygonTileCache manages cached polygon layer tiles
type PolygonTileCache struct {
	cache    map[int]map[int]map[int]*PolygonTile // [zoom][x][y]
	lruList  *list.List
	lruMap   map[string]*list.Element
	maxTiles int
	mu       sync.RWMutex
}

func NewPolygonTileCache(maxTiles int) *PolygonTileCache {
	return &PolygonTileCache{
		cache:    make(map[int]map[int]map[int]*PolygonTile),
		lruList:  list.New(),
		lruMap:   make(map[string]*list.Element),
		maxTiles: maxTiles,
	}
}

func (g *Game) getPolygonTile(tileX, tileY, zoom int) *PolygonTile {
	g.PolygonTileCache.mu.RLock()
	if zoomLevel, exists := g.PolygonTileCache.cache[zoom]; exists {
		if xLevel, exists := zoomLevel[tileX]; exists {
			if tile, exists := xLevel[tileY]; exists {
				g.PolygonTileCache.mu.RUnlock()
				return tile
			}
		}
	}
	g.PolygonTileCache.mu.RUnlock()

	// Create new tile
	tile := g.renderPolygonTile(tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Cache the tile
	g.PolygonTileCache.mu.Lock()
	defer g.PolygonTileCache.mu.Unlock()

	if _, exists := g.PolygonTileCache.cache[zoom]; !exists {
		g.PolygonTileCache.cache[zoom] = make(map[int]map[int]*PolygonTile)
	}
	if _, exists := g.PolygonTileCache.cache[zoom][tileX]; !exists {
		g.PolygonTileCache.cache[zoom][tileX] = make(map[int]*PolygonTile)
	}
	g.PolygonTileCache.cache[zoom][tileX][tileY] = tile

	return tile
}

func getTileBounds(tileX, tileY, zoom int) Bounds {
	pixelX := float64(tileX * tileSizePixels)
	pixelY := float64(tileY * tileSizePixels)

	// Convert directly to WGS84 bounds
	minLat, minLon := pixelToLatLng(pixelX, pixelY+tileSizePixels, zoom)
	maxLat, maxLon := pixelToLatLng(pixelX+tileSizePixels, pixelY, zoom)

	return Bounds{
		MinX: minLon,
		MinY: minLat,
		MaxX: maxLon,
		MaxY: maxLat,
	}
}

// In polygons.go, modify renderPolygonTile():
func (g *Game) renderPolygonTile(tileX, tileY, zoom int) *PolygonTile {
	bounds := getTileBounds(tileX, tileY, zoom)
	tile := &PolygonTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	polygons := g.PolygonLayer.Index.Search(bounds)
	fmt.Printf("Found %d polygons in tile bounds\n", len(polygons))

	tileOriginX := float64(tileX * tileSizePixels)
	tileOriginY := float64(tileY * tileSizePixels)

	for _, p := range polygons {
		polygon := p.(*Polygon)
		if len(polygon.Points) < 3 {
			continue
		}

		// Convert points to vertices
		vertices := make([]ebiten.Vertex, len(polygon.Points))
		for i, pt := range polygon.Points {
			x, y := latLngToPixel(pt.Lat, pt.Lon, zoom)
			x -= tileOriginX
			y -= tileOriginY

			vertices[i] = ebiten.Vertex{
				DstX:   float32(x),
				DstY:   float32(y),
				SrcX:   1,
				SrcY:   1,
				ColorR: 0,
				ColorG: 1,
				ColorB: 0,
				ColorA: 0.5,
			}
		}

		// Use ear clipping triangulation
		indices := triangulatePolygon(polygon.Points)

		// Draw the filled polygon
		tile.Image.DrawTriangles(vertices, indices, whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), nil)
	}

	return tile
}

// DrawPolygons renders visible polygon tiles
func (g *Game) DrawPolygons(screen *ebiten.Image) {
	if !g.PolygonLayer.Visible {
		return
	}

	// Calculate visible tile range
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	topLeftX := centerX - float64(g.ScreenWidth)/2
	topLeftY := centerY - float64(g.ScreenHeight)/2

	startTileX := int(math.Floor(topLeftX / tileSizePixels))
	startTileY := int(math.Floor(topLeftY / tileSizePixels))

	tilesX := int(math.Ceil(float64(g.ScreenWidth)/tileSizePixels)) + 2
	tilesY := int(math.Ceil(float64(g.ScreenHeight)/tileSizePixels)) + 2

	// Draw visible tiles
	for x := 0; x < tilesX; x++ {
		for y := 0; y < tilesY; y++ {
			tileX := startTileX + x
			tileY := startTileY + y

			tile := g.getPolygonTile(tileX, tileY, g.zoom)
			if tile == nil {
				continue
			}

			screenX := float64(tileX*tileSizePixels) - topLeftX
			screenY := float64(tileY*tileSizePixels) - topLeftY

			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(screenX, screenY)
			screen.DrawImage(tile.Image, op)
		}
	}
}

func (g *Game) clearAffectedPolygonTiles(polygon *Polygon) {
	bounds := polygon.Bounds()
	g.PolygonTileCache.mu.Lock()
	defer g.PolygonTileCache.mu.Unlock()

	// For each zoom level in cache
	for zoom := range g.PolygonTileCache.cache {
		minX, minY := latLngToPixel(bounds.MinY, bounds.MinX, zoom)
		maxX, maxY := latLngToPixel(bounds.MaxY, bounds.MaxX, zoom)

		if minX > maxX {
			minX, maxX = maxX, minX
		}
		if minY > maxY {
			minY, maxY = maxY, minY
		}

		minTileX := int(math.Floor(minX / tileSizePixels))
		maxTileX := int(math.Floor(maxX / tileSizePixels))
		minTileY := int(math.Floor(minY / tileSizePixels))
		maxTileY := int(math.Floor(maxY / tileSizePixels))

		for tileX := minTileX; tileX <= maxTileX; tileX++ {
			if _, exists := g.PolygonTileCache.cache[zoom][tileX]; exists {
				for tileY := minTileY; tileY <= maxTileY; tileY++ {
					delete(g.PolygonTileCache.cache[zoom][tileX], tileY)
				}
			}
		}
	}
	g.needRedraw = true
}

func isEar(p1, p2, p3 Point, points []Point) bool {
	// First check if triangle is oriented counterclockwise
	area := ((p2.Lon - p1.Lon) * (p3.Lat - p1.Lat)) -
		((p3.Lon - p1.Lon) * (p2.Lat - p1.Lat))
	if area <= 0 {
		return false
	}

	// Then check if any point is inside this triangle
	for _, p := range points {
		if p == p1 || p == p2 || p == p3 {
			continue
		}

		// Use barycentric coordinates to check if point is inside triangle
		alpha := ((p2.Lat-p3.Lat)*(p.Lon-p3.Lon) +
			(p3.Lon-p2.Lon)*(p.Lat-p3.Lat)) /
			((p2.Lat-p3.Lat)*(p1.Lon-p3.Lon) +
				(p3.Lon-p2.Lon)*(p1.Lat-p3.Lat))

		beta := ((p3.Lat-p1.Lat)*(p.Lon-p3.Lon) +
			(p1.Lon-p3.Lon)*(p.Lat-p3.Lat)) /
			((p2.Lat-p3.Lat)*(p1.Lon-p3.Lon) +
				(p3.Lon-p2.Lon)*(p1.Lat-p3.Lat))

		gamma := 1.0 - alpha - beta

		if alpha > 0 && beta > 0 && gamma > 0 {
			return false
		}
	}
	return true
}

// Update triangulatePolygon to ensure proper vertex ordering
func triangulatePolygon(points []Point) []uint16 {
	if len(points) < 3 {
		return nil
	}

	// Create a working copy of the points
	vertices := make([]int, len(points))
	for i := range vertices {
		vertices[i] = i
	}

	var indices []uint16
	n := len(vertices)

	// Main ear cutting loop
	for n > 3 {
		foundEar := false
		for i := 0; i < n; i++ {
			prev := vertices[(i+n-1)%n]
			curr := vertices[i]
			next := vertices[(i+1)%n]

			if isEar(points[prev], points[curr], points[next], points) {
				// Add triangle indices
				indices = append(indices, uint16(prev), uint16(curr), uint16(next))

				// Remove current vertex
				for j := i; j < n-1; j++ {
					vertices[j] = vertices[j+1]
				}
				n--
				foundEar = true
				break
			}
		}

		if !foundEar {
			// No ear found - add remaining vertices as a fan
			for i := 1; i < n-1; i++ {
				indices = append(indices,
					uint16(vertices[0]),
					uint16(vertices[i]),
					uint16(vertices[i+1]))
			}
			break
		}
	}

	// Add final triangle if needed
	if n == 3 {
		indices = append(indices,
			uint16(vertices[0]),
			uint16(vertices[1]),
			uint16(vertices[2]))
	}

	return indices
}
