// lines.go
package main

import (
	"container/list"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// LineTile represents a cached rendering of lines for a specific tile
type LineTile struct {
	Image     *ebiten.Image
	Bounds    Bounds
	ZoomLevel int
}

// LineTileCache manages cached line layer tiles
type LineTileCache struct {
	cache    map[int]map[int]map[int]*LineTile // [zoom][x][y]
	lruList  *list.List
	lruMap   map[string]*list.Element
	maxTiles int
	mu       sync.RWMutex
}

const (
	lineWidth     = 2.0
	feetPerDegree = 364000 // approximate feet per degree at 40°N
)

// Add new functions
func degreesFromFeet(feet float64) float64 {
	return feet / feetPerDegree
}

func NewLineTileCache(maxTiles int) *LineTileCache {
	return &LineTileCache{
		cache:    make(map[int]map[int]map[int]*LineTile),
		lruList:  list.New(),
		lruMap:   make(map[string]*list.Element),
		maxTiles: maxTiles,
	}
}

// getLineTileBounds calculates bounds with padding for line width
func getLineTileBounds(tileX, tileY, zoom int) Bounds {
	pixelX := float64(tileX * tileSizePixels)
	pixelY := float64(tileY * tileSizePixels)

	padPixels := lineWidth

	minLat, minLon := pixelToLatLng(pixelX-padPixels, pixelY+tileSizePixels+padPixels, zoom)
	maxLat, maxLon := pixelToLatLng(pixelX+tileSizePixels+padPixels, pixelY-padPixels, zoom)

	return Bounds{
		MinX: minLon,
		MinY: minLat,
		MaxX: maxLon,
		MaxY: maxLat,
	}
}

func (c *LineTileCache) get(zoom, x, y int) *LineTile {
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

func (c *LineTileCache) set(zoom, x, y int, tile *LineTile) {
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
		c.cache[zoom] = make(map[int]map[int]*LineTile)
	}
	if _, ok := c.cache[zoom][x]; !ok {
		c.cache[zoom][x] = make(map[int]*LineTile)
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

func (g *Goliath) getLineTile(layer *Layer, tileX, tileY, zoom int) *LineTile {
	// Skip tile creation if zoom is changing rapidly
	if !g.isZoomStable() {
		return nil
	}

	bounds := getTileBoundsWithPadding(tileX, tileY, zoom)

	// Skip if no geometry in tile bounds
	if !layer.HasGeometryInTileBounds(bounds) {
		return nil
	}

	layer.LineTileCache.mu.RLock()
	tile := layer.LineTileCache.get(zoom, tileX, tileY)
	if tile != nil {
		layer.LineTileCache.mu.RUnlock()
		return tile
	}
	layer.LineTileCache.mu.RUnlock()

	// Create new tile
	tile = g.renderLineTile(layer, tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Store with proper LRU handling
	layer.LineTileCache.mu.Lock()
	layer.LineTileCache.set(zoom, tileX, tileY, tile)
	layer.LineTileCache.mu.Unlock()

	return tile
}

// renderLineTile renders lines within a tile
func (g *Goliath) renderLineTile(layer *Layer, tileX, tileY, zoom int) *LineTile {
	bounds := getLineTileBounds(tileX, tileY, zoom)
	tile := &LineTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	lines := layer.PolylineLayer.Index.Search(bounds)
	tileOriginX := float64(tileX * tileSizePixels)
	tileOriginY := float64(tileY * tileSizePixels)

	for _, l := range lines {
		line := l.(*LineString)
		if len(line.Points) < 2 {
			continue
		}

		// Set line style based on selection state
		var lineColor color.RGBA
		var lineThickness float32

		if g.defaultRender {
			if line.Selected {
				lineColor = color.RGBA{255, 255, 0, 255} // Yellow for selected
				lineThickness = float32(lineWidth * 1.5)
			} else {
				lineColor = color.RGBA{0, 0, 255, 255} // Blue for default
				lineThickness = float32(lineWidth)
			}
		} else {
			if line.Selected {
				lineColor = color.RGBA{255, 255, 0, 255}
				lineThickness = float32(lineWidth * 1.5)
			} else {
				if line.Color == (color.RGBA{}) {
					lineColor = color.RGBA{0, 0, 255, 255}
				} else {
					lineColor = line.Color
				}
				if line.Width == 0 {
					lineThickness = float32(lineWidth)
				} else {
					lineThickness = line.Width
				}
			}
		}

		// Draw line segments
		for i := 0; i < len(line.Points)-1; i++ {
			x1, y1 := latLngToPixel(line.Points[i].Lat, line.Points[i].Lon, zoom)
			x2, y2 := latLngToPixel(line.Points[i+1].Lat, line.Points[i+1].Lon, zoom)

			x1 -= tileOriginX
			y1 -= tileOriginY
			x2 -= tileOriginX
			y2 -= tileOriginY

			// Draw with selection-based style
			vector.StrokeLine(tile.Image,
				float32(x1), float32(y1),
				float32(x2), float32(y2),
				lineThickness, lineColor, false)
		}
	}

	return tile
}

// DrawLines renders visible line tiles
func (g *Goliath) DrawLines(screen *ebiten.Image) {
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

					tile := g.getLineTile(layer, tileX, tileY, g.zoom)
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

func randomLineString(startLat, startLon float64) *LineString {
	numPoints := 2 + rand.Intn(11) // 2-12 points
	points := make([]Point, numPoints)
	points[0] = Point{Lat: startLat, Lon: startLon}

	for i := 1; i < numPoints; i++ {
		// Random bearing 0-360°
		bearing := rand.Float64() * 360.0
		// Random distance 100-500 feet
		distance := 1000.0 + rand.Float64()*4000.0

		// Convert bearing and distance to lat/lon delta
		distDegrees := degreesFromFeet(distance)
		bearingRad := bearing * math.Pi / 180.0

		dLat := distDegrees * math.Cos(bearingRad)
		dLon := distDegrees * math.Sin(bearingRad) / math.Cos(points[i-1].Lat*math.Pi/180.0)

		points[i] = Point{
			Lat: points[i-1].Lat + dLat,
			Lon: points[i-1].Lon + dLon,
		}
	}

	return &LineString{Points: points}
}

func (g *Goliath) InitializeTestLines(layer *Layer, numLines int) {
	const numWorkers = 10

	// Create channels
	jobs := make(chan int, numLines)
	results := make(chan *LineString, numLines)
	var wg sync.WaitGroup

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				startLat := 26.0 + rand.Float64()*(47.0-26.0)
				startLon := -123.0 + rand.Float64()*(-76.0-(-123.0))
				line := randomLineString(startLat, startLon)
				results <- line
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < numLines; i++ {
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
	for line := range results {
		layer.PolylineLayer.Index.Insert(line, line.Bounds())
		count++
		if count%1000 == 0 {
			fmt.Printf("Generated %d lines...\n", count)
		}
	}

	fmt.Printf("Added %d lines to R-tree\n", count)

	// Clear line tile cache
	layer.LineTileCache.mu.Lock()
	layer.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
	layer.LineTileCache.lruList = list.New()
	layer.LineTileCache.lruMap = make(map[string]*list.Element)
	layer.LineTileCache.mu.Unlock()

	g.needRedraw = true
}

func (g *Goliath) clearAffectedLineTiles(layer *Layer, line *LineString) {
	layer.invalidateBounds()

	bounds := line.Bounds()
	layer.LineTileCache.mu.Lock()
	defer layer.LineTileCache.mu.Unlock()

	// For each zoom level in cache
	for zoom := range layer.LineTileCache.cache {
		// Fix coordinate order: latLngToPixel expects (lat, lon)
		minX, minY := latLngToPixel(bounds.MinY, bounds.MinX, zoom)
		maxX, maxY := latLngToPixel(bounds.MaxY, bounds.MaxX, zoom)

		// Swap if needed to ensure correct range
		if minX > maxX {
			minX, maxX = maxX, minX
		}
		if minY > maxY {
			minY, maxY = maxY, minY
		}

		// Calculate affected tile range
		minTileX := int(math.Floor(minX / tileSizePixels))
		maxTileX := int(math.Floor(maxX / tileSizePixels))
		minTileY := int(math.Floor(minY / tileSizePixels))
		maxTileY := int(math.Floor(maxY / tileSizePixels))

		// Remove affected tiles
		for tileX := minTileX; tileX <= maxTileX; tileX++ {
			if _, exists := layer.LineTileCache.cache[zoom][tileX]; exists {
				for tileY := minTileY; tileY <= maxTileY; tileY++ {
					delete(layer.LineTileCache.cache[zoom][tileX], tileY)
				}
			}
		}
	}
	g.needRedraw = true
}
