// polygon.go
package main

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/flywave/go-earcut"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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

func (c *PolygonTileCache) get(zoom, x, y int) *PolygonTile {
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)

	if zoomLevel, exists := c.cache[zoom]; exists {
		if xLevel, exists := zoomLevel[x]; exists {
			if tile, exists := xLevel[y]; exists {
				// Update LRU on access
				if elem, exists := c.lruMap[key]; exists {
					c.lruList.MoveToFront(elem)
				}
				return tile
			}
		}
	}
	return nil
}

func (c *PolygonTileCache) set(zoom, x, y int, tile *PolygonTile) {
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)

	// Handle eviction if we're at capacity
	if c.lruList.Len() >= c.maxTiles {
		oldest := c.lruList.Back()
		if oldest != nil {
			oldKey := oldest.Value.(string)
			var oz, ox, oy int
			fmt.Sscanf(oldKey, "%d-%d-%d", &oz, &ox, &oy)

			// Remove from cache
			if _, exists := c.cache[oz]; exists {
				if _, exists := c.cache[oz][ox]; exists {
					delete(c.cache[oz][ox], oy)
					if len(c.cache[oz][ox]) == 0 {
						delete(c.cache[oz], ox)
					}
				}
				if len(c.cache[oz]) == 0 {
					delete(c.cache, oz)
				}
			}

			// Remove from LRU
			c.lruList.Remove(oldest)
			delete(c.lruMap, oldKey)
		}
	}

	// Add to cache
	if _, ok := c.cache[zoom]; !ok {
		c.cache[zoom] = make(map[int]map[int]*PolygonTile)
	}
	if _, ok := c.cache[zoom][x]; !ok {
		c.cache[zoom][x] = make(map[int]*PolygonTile)
	}
	c.cache[zoom][x][y] = tile

	// Update LRU
	if elem, exists := c.lruMap[key]; exists {
		c.lruList.MoveToFront(elem)
	} else {
		elem := c.lruList.PushFront(key)
		c.lruMap[key] = elem
	}
}

func (g *Game) getPolygonTile(layer *Layer, tileX, tileY, zoom int) *PolygonTile {
	// Skip tile creation if zoom is changing rapidly
	if !g.isZoomStable() {
		return nil
	}

	bounds := getTileBoundsWithPadding(tileX, tileY, zoom)

	// Skip if no geometry in tile bounds
	if !layer.HasGeometryInTileBounds(bounds) {
		return nil
	}

	layer.PolygonTileCache.mu.RLock()
	tile := layer.PolygonTileCache.get(zoom, tileX, tileY)
	if tile != nil {
		layer.PolygonTileCache.mu.RUnlock()
		return tile
	}
	layer.PolygonTileCache.mu.RUnlock()

	// Create new tile
	tile = g.renderPolygonTile(layer, tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Store with proper LRU handling
	layer.PolygonTileCache.mu.Lock()
	layer.PolygonTileCache.set(zoom, tileX, tileY, tile)
	layer.PolygonTileCache.mu.Unlock()

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

func (g *Game) renderPolygonTile(layer *Layer, tileX, tileY, zoom int) *PolygonTile {
	bounds := getTileBounds(tileX, tileY, zoom)
	tile := &PolygonTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	polygons := layer.PolygonLayer.Index.Search(bounds)
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

			if polygon.Selected {
				// Selected polygon: yellow with higher opacity
				vertices[i] = ebiten.Vertex{
					DstX:   float32(x),
					DstY:   float32(y),
					SrcX:   1,
					SrcY:   1,
					ColorR: 1.0, // Red
					ColorG: 1.0, // Green
					ColorB: 0.0, // Blue = 0 for yellow
					ColorA: 0.7, // Higher opacity when selected
				}
			} else {
				// Normal state: green with lower opacity
				vertices[i] = ebiten.Vertex{
					DstX:   float32(x),
					DstY:   float32(y),
					SrcX:   1,
					SrcY:   1,
					ColorR: 0.0,
					ColorG: 1.0,
					ColorB: 0.0,
					ColorA: 0.3,
				}
			}
		}

		// Use ear clipping triangulation
		indices := triangulatePolygon(polygon.Points)

		// Draw the filled polygon
		tile.Image.DrawTriangles(vertices, indices, whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), nil)

		// Draw outline
		for i := 0; i < len(polygon.Points); i++ {
			p1 := polygon.Points[i]
			p2 := polygon.Points[(i+1)%len(polygon.Points)]
			x1, y1 := latLngToPixel(p1.Lat, p1.Lon, zoom)
			x2, y2 := latLngToPixel(p2.Lat, p2.Lon, zoom)
			x1 -= tileOriginX
			y1 -= tileOriginY
			x2 -= tileOriginX
			y2 -= tileOriginY

			lineWidth := float32(1.0)
			if polygon.Selected {
				lineWidth = 2.0 // Thicker outline when selected
			}

			// Draw polygon outline
			vector.StrokeLine(tile.Image,
				float32(x1), float32(y1),
				float32(x2), float32(y2),
				lineWidth,
				color.RGBA{0, 0, 0, 255},
				false)
		}
	}

	return tile
}

// DrawPolygons renders visible polygon tiles
func (g *Game) DrawPolygons(screen *ebiten.Image) {
	visibleBounds := g.getVisibleBounds()

	for _, rootLayer := range g.layers {
		WalkLayers(rootLayer, func(layer *Layer) {
			if !layer.IsEffectivelyVisible() {
				return
			}

			// Skip layer if no geometry in view
			if !layer.HasGeometryInView(visibleBounds) {
				return
			}

			centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
			topLeftX := centerX - float64(g.ScreenWidth)/2
			topLeftY := centerY - float64(g.ScreenHeight)/2

			startTileX := int(math.Floor(topLeftX / tileSizePixels))
			startTileY := int(math.Floor(topLeftY / tileSizePixels))

			tilesX := int(math.Ceil(float64(g.ScreenWidth)/tileSizePixels)) + 2
			tilesY := int(math.Ceil(float64(g.ScreenHeight)/tileSizePixels)) + 2

			for x := 0; x < tilesX; x++ {
				for y := 0; y < tilesY; y++ {
					tileX := startTileX + x
					tileY := startTileY + y

					tile := g.getPolygonTile(layer, tileX, tileY, g.zoom)
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
		})
	}
}

func (g *Game) clearAffectedPolygonTiles(layer *Layer, polygon *Polygon) {
	layer.invalidateBounds()

	bounds := polygon.Bounds()
	layer.PolygonTileCache.mu.Lock()
	defer layer.PolygonTileCache.mu.Unlock()

	// For each zoom level in cache
	for zoom := range layer.PolygonTileCache.cache {
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
			if _, exists := layer.PolygonTileCache.cache[zoom][tileX]; exists {
				for tileY := minTileY; tileY <= maxTileY; tileY++ {
					delete(layer.PolygonTileCache.cache[zoom][tileX], tileY)
				}
			}
		}
	}
	g.needRedraw = true
}

func triangulatePolygon(points []Point) []uint16 {
	if len(points) < 3 {
		return nil
	}

	// Flatten the points for earcut
	coords := make([]float64, len(points)*2)
	for i, p := range points {
		coords[i*2] = p.Lon   // x coordinate
		coords[i*2+1] = p.Lat // y coordinate
	}

	// Call the earcut implementation
	indices, err := earcut.Earcut(coords, nil, 2)
	if err != nil {
		log.Printf("Failed to triangulate polygon: %v", err)
		return nil
	}

	// Convert to uint16
	uint16Indices := make([]uint16, len(indices))
	for i, idx := range indices {
		uint16Indices[i] = uint16(idx)
	}

	return uint16Indices
}

func randomPolygon(startLat, startLon float64) *Polygon {
	numVertices := 3 + rand.Intn(10) // 3-12 vertices

	// Generate a random radius between 1000-20000 feet
	baseRadius := 1000.0 + rand.Float64()*19000.0

	// Create points array
	points := make([]Point, numVertices)
	points[0] = Point{Lat: startLat, Lon: startLon}

	// Generate vertices in a circle with some random variation
	angleStep := 360.0 / float64(numVertices)
	for i := 0; i < numVertices; i++ {
		angle := float64(i) * angleStep

		// Add small random variation to radius (but not too much to maintain convexity)
		radius := baseRadius * (0.8 + 0.4*rand.Float64())

		// Convert to radians
		angleRad := angle * math.Pi / 180.0

		// Convert radius to degrees
		distDegrees := degreesFromFeet(radius)

		// Calculate offset
		dLat := distDegrees * math.Cos(angleRad)
		dLon := distDegrees * math.Sin(angleRad) / math.Cos(startLat*math.Pi/180.0)

		points[i] = Point{
			Lat: startLat + dLat,
			Lon: startLon + dLon,
		}
	}

	return &Polygon{Points: points}
}

func (g *Game) InitializeTestPolygons(layer *Layer, numPolygons int) {
	const numWorkers = 10

	// Create channels
	jobs := make(chan int, numPolygons)
	results := make(chan *Polygon, numPolygons)
	var wg sync.WaitGroup

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				startLat := 26.0 + rand.Float64()*(47.0-26.0)
				startLon := -123.0 + rand.Float64()*(-76.0-(-123.0))
				polygon := randomPolygon(startLat, startLon)
				results <- polygon
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < numPolygons; i++ {
			jobs <- i
		}
		close(jobs)
	}()

	// Start collector
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	count := 0
	for polygon := range results {
		layer.PolygonLayer.Index.Insert(polygon, polygon.Bounds())
		count++
		if count%1000 == 0 {
			fmt.Printf("Generated %d polygons...\n", count)
		}
	}

	fmt.Printf("Added %d polygons to R-tree\n", count)

	// Clear polygon tile cache
	layer.PolygonTileCache.mu.Lock()
	layer.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
	layer.PolygonTileCache.lruList = list.New()
	layer.PolygonTileCache.lruMap = make(map[string]*list.Element)
	layer.PolygonTileCache.mu.Unlock()

	g.needRedraw = true
}
