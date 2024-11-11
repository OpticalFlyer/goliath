// points.go
package main

import (
	"container/list"
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/jonas-p/go-shp"
)

const (
	pointSpriteSize = 10
	tileSizePixels  = 256
)

// PointTile represents a cached rendering of points for a specific tile
type PointTile struct {
	Image     *ebiten.Image
	Bounds    Bounds
	ZoomLevel int
}

// PointTileCache manages cached point layer tiles
type PointTileCache struct {
	cache    map[int]map[int]map[int]*PointTile // [zoom][x][y]
	lruList  *list.List
	lruMap   map[string]*list.Element
	maxTiles int
	mu       sync.RWMutex
}

var (
	pointSprite *ebiten.Image
)

func init() {
	// Create point sprite - white filled circle with black border
	pointSprite = ebiten.NewImage(pointSpriteSize, pointSpriteSize)

	// Draw white fill
	vector.DrawFilledCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, color.White, false)

	// Draw black border
	vector.StrokeCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, 1, color.Black, false)
}

// InitializeTestPoints adds random points in parallel using worker pools
func (g *Game) InitializeTestPoints(numPoints int) {
	const numWorkers = 10

	// Continental US bounds approximately
	minLat := 26.000000
	maxLat := 47.000000
	minLon := -123.000000
	maxLon := -76.000000

	// Create channels
	jobs := make(chan int, numPoints)
	results := make(chan *Point, numPoints)
	var wg sync.WaitGroup

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				lat := minLat + rand.Float64()*(maxLat-minLat)
				lon := minLon + rand.Float64()*(maxLon-minLon)
				point := NewPoint(lat, lon)
				results <- point
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < numPoints; i++ {
			jobs <- i
		}
		close(jobs)
	}()

	// Start point collector
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and insert points
	count := 0
	for point := range results {
		g.PointLayer.Index.Insert(point, point.Bounds())
		count++
		if count%100000 == 0 {
			fmt.Printf("Generated %d points...\n", count)
		}
	}

	fmt.Printf("Added %d points to R-tree\n", g.PointLayer.Index.Size)

	// Clear entire point tile cache
	g.PointTileCache.mu.Lock()
	g.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	g.PointTileCache.lruList = list.New()
	g.PointTileCache.lruMap = make(map[string]*list.Element)
	g.PointTileCache.mu.Unlock()

	g.needRedraw = true
}

func NewPointTileCache(maxTiles int) *PointTileCache {
	return &PointTileCache{
		cache:    make(map[int]map[int]map[int]*PointTile),
		lruList:  list.New(),
		lruMap:   make(map[string]*list.Element),
		maxTiles: maxTiles,
	}
}

// Calculate tile bounds with padding for point sprites
func getTileBoundsWithPadding(tileX, tileY, zoom int) Bounds {
	// Convert tile coordinates to pixel coordinates
	pixelX := float64(tileX * tileSizePixels)
	pixelY := float64(tileY * tileSizePixels)

	// Add padding for point sprites
	padPixels := float64(pointSpriteSize)

	// Convert directly to WGS84 bounds
	minLat, minLon := pixelToLatLng(pixelX-padPixels, pixelY+tileSizePixels+padPixels, zoom)
	maxLat, maxLon := pixelToLatLng(pixelX+tileSizePixels+padPixels, pixelY-padPixels, zoom)

	return Bounds{
		MinX: minLon,
		MinY: minLat,
		MaxX: maxLon,
		MaxY: maxLat,
	}
}

// Render points for a specific tile
func (g *Game) renderPointTile(tileX, tileY, zoom int) *PointTile {
	bounds := getTileBoundsWithPadding(tileX, tileY, zoom)
	tile := &PointTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	// Get points within padded bounds
	points := g.PointLayer.Index.Search(bounds)
	//log.Printf("Found %d points within bounds for tileX=%d, tileY=%d, zoom=%d", len(points), tileX, tileY, zoom)

	// Calculate tile origin in pixel coordinates
	tileOriginX := float64(tileX * tileSizePixels)
	tileOriginY := float64(tileY * tileSizePixels)

	// Draw points
	for _, p := range points {
		point := p.(*Point)
		worldX, worldY := latLngToPixel(point.Lat, point.Lon, zoom)

		// Convert to tile-local coordinates
		localX := worldX - tileOriginX - float64(pointSpriteSize)/2
		localY := worldY - tileOriginY - float64(pointSpriteSize)/2

		// Draw if point sprite intersects tile
		if localX > -pointSpriteSize && localX < tileSizePixels &&
			localY > -pointSpriteSize && localY < tileSizePixels {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(localX, localY)
			tile.Image.DrawImage(pointSprite, op)
		}
	}

	//log.Printf("Tile rendered for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, zoom)
	return tile
}

// Modified DrawPoints to use tile cache
func (g *Game) DrawPoints(screen *ebiten.Image) {
	if !g.PointLayer.Visible {
		return
	}

	//log.Println("DrawPoints called")

	// Calculate visible tile range
	centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
	topLeftX := centerX - float64(g.ScreenWidth)/2
	topLeftY := centerY - float64(g.ScreenHeight)/2

	startTileX := int(math.Floor(topLeftX / tileSizePixels))
	startTileY := int(math.Floor(topLeftY / tileSizePixels))

	tilesX := int(math.Ceil(float64(g.ScreenWidth)/tileSizePixels)) + 2
	tilesY := int(math.Ceil(float64(g.ScreenHeight)/tileSizePixels)) + 2

	//log.Printf("Visible tile range: startTileX=%d, startTileY=%d, tilesX=%d, tilesY=%d", startTileX, startTileY, tilesX, tilesY)

	// Draw visible tiles
	for x := 0; x < tilesX; x++ {
		for y := 0; y < tilesY; y++ {
			tileX := startTileX + x
			tileY := startTileY + y

			// Get or create tile
			tile := g.getPointTile(tileX, tileY, g.zoom)
			if tile == nil {
				log.Printf("Failed to get or create tile for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, g.zoom)
				continue
			}

			// Calculate screen position
			screenX := float64(tileX*tileSizePixels) - topLeftX
			screenY := float64(tileY*tileSizePixels) - topLeftY

			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(screenX, screenY)
			screen.DrawImage(tile.Image, op)
		}
	}
}

// Get or create point tile
func (g *Game) getPointTile(tileX, tileY, zoom int) *PointTile {
	//log.Printf("getPointTile called for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, zoom)

	// First check cache with read lock
	g.PointTileCache.mu.RLock()
	if tile := g.PointTileCache.get(zoom, tileX, tileY); tile != nil {
		g.PointTileCache.mu.RUnlock()
		//log.Printf("Tile found in cache for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, zoom)
		return tile
	}
	g.PointTileCache.mu.RUnlock()

	// Create new tile without holding any locks
	tile := g.renderPointTile(tileX, tileY, zoom)
	if tile == nil {
		//log.Printf("Failed to render tile for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, zoom)
		return nil
	}

	// Try to store in cache with write lock
	g.PointTileCache.mu.Lock()
	g.PointTileCache.set(zoom, tileX, tileY, tile)
	g.PointTileCache.mu.Unlock()

	//log.Printf("Tile created and cached for tileX=%d, tileY=%d, zoom=%d", tileX, tileY, zoom)
	return tile
}

// get retrieves a tile from the cache
func (c *PointTileCache) get(zoom, x, y int) *PointTile {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if zoom level exists
	if zoomLevel, exists := c.cache[zoom]; exists {
		// Check if x coordinate exists
		if xLevel, exists := zoomLevel[x]; exists {
			// Check if y coordinate exists
			if tile, exists := xLevel[y]; exists {
				// Update LRU
				key := fmt.Sprintf("%d-%d-%d", zoom, x, y)
				if element, exists := c.lruMap[key]; exists {
					c.lruList.MoveToFront(element)
				}
				return tile
			}
		}
	}
	return nil
}

// set adds a tile to the cache with LRU eviction
func (c *PointTileCache) set(zoom, x, y int, tile *PointTile) {
	// Ensure zoom level map exists
	if _, exists := c.cache[zoom]; !exists {
		c.cache[zoom] = make(map[int]map[int]*PointTile)
	}
	// Ensure x coordinate map exists
	if _, exists := c.cache[zoom][x]; !exists {
		c.cache[zoom][x] = make(map[int]*PointTile)
	}

	// Add tile to cache
	c.cache[zoom][x][y] = tile

	// Update LRU
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)
	if element, exists := c.lruMap[key]; exists {
		c.lruList.MoveToFront(element)
	} else {
		// Add new entry to LRU
		element := c.lruList.PushFront(key)
		c.lruMap[key] = element

		// Check if we need to evict
		if c.lruList.Len() > c.maxTiles {
			// Remove oldest entry
			oldest := c.lruList.Back()
			if oldest != nil {
				oldestKey := oldest.Value.(string)
				c.lruList.Remove(oldest)
				delete(c.lruMap, oldestKey)

				// Parse key to get coordinates
				var z, x, y int
				fmt.Sscanf(oldestKey, "%d-%d-%d", &z, &x, &y)

				// Remove from cache
				delete(c.cache[z][x], y)
				if len(c.cache[z][x]) == 0 {
					delete(c.cache[z], x)
				}
				if len(c.cache[z]) == 0 {
					delete(c.cache, z)
				}
			}
		}
	}
}

// clearAffectedTiles removes cached tiles that contain the given point
func (g *Game) clearAffectedTiles(point *Point) {
	g.PointTileCache.mu.Lock()
	defer g.PointTileCache.mu.Unlock()

	// For each zoom level in the cache
	for zoom := range g.PointTileCache.cache {
		// Convert point coordinates to pixel coordinates at this zoom
		pixelX, pixelY := latLngToPixel(point.Lat, point.Lon, zoom)

		// Calculate tile coordinates (considering padding for sprite overlap)
		padding := float64(pointSpriteSize)

		// Calculate affected tile range
		minTileX := int(math.Floor((pixelX - padding) / tileSizePixels))
		maxTileX := int(math.Floor((pixelX + padding) / tileSizePixels))
		minTileY := int(math.Floor((pixelY - padding) / tileSizePixels))
		maxTileY := int(math.Floor((pixelY + padding) / tileSizePixels))

		// Remove affected tiles
		for tileX := minTileX; tileX <= maxTileX; tileX++ {
			for tileY := minTileY; tileY <= maxTileY; tileY++ {
				if xLevel, exists := g.PointTileCache.cache[zoom][tileX]; exists {
					if _, exists := xLevel[tileY]; exists {
						// Remove from LRU
						key := fmt.Sprintf("%d-%d-%d", zoom, tileX, tileY)
						if element, exists := g.PointTileCache.lruMap[key]; exists {
							g.PointTileCache.lruList.Remove(element)
							delete(g.PointTileCache.lruMap, key)
						}
						// Remove tile
						delete(xLevel, tileY)
					}
				}
			}
		}
	}
}

func (g *Game) loadShapefile(path string) {
	fmt.Printf("Loading shapefile: %s\n", path)

	// Open shapefile
	shapeFile, err := shp.Open(path)
	if err != nil {
		fmt.Printf("Error opening shapefile: %v\n", err)
		return
	}
	defer shapeFile.Close()

	// Check if it's a point shapefile
	if shapeFile.Next() {
		_, shape := shapeFile.Shape()
		if _, ok := shape.(*shp.Point); !ok {
			fmt.Printf("Not a point shapefile\n")
			return
		}
		// Reset reader
		shapeFile.Close()
		shapeFile, _ = shp.Open(path)
	}

	// Create channels for concurrent processing
	const numWorkers = 10
	jobs := make(chan shp.Shape, 1000)
	var wg sync.WaitGroup
	count := atomic.Int64{}

	var cacheClearMutex sync.Mutex
	lastCacheClear := atomic.Int64{}

	// Launch workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localCount := 0
			for shape := range jobs {
				point := shape.(*shp.Point)
				p := NewPoint(point.Y, point.X)

				// Insert directly in worker
				g.PointLayer.Index.Insert(p, p.Bounds())

				localCount++
				newCount := count.Add(1)

				// Check if we need to clear cache
				if newCount/100000 > lastCacheClear.Load() {
					cacheClearMutex.Lock()
					if newCount/100000 > lastCacheClear.Load() {
						// Clear point tile cache
						g.PointTileCache.mu.Lock()
						g.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
						g.PointTileCache.lruList = list.New()
						g.PointTileCache.lruMap = make(map[string]*list.Element)
						g.PointTileCache.mu.Unlock()

						lastCacheClear.Store(newCount / 100000)
						g.needRedraw = true
						fmt.Printf("Cleared cache after %d points\n", newCount)
					}
					cacheClearMutex.Unlock()
				}

				if newCount%10000 == 0 {
					fmt.Printf("Loaded %d points...\n", newCount)
				}
			}
		}()
	}

	// Start sender
	go func() {
		for shapeFile.Next() {
			_, shape := shapeFile.Shape()
			jobs <- shape
		}
		close(jobs)
	}()

	// Wait for completion
	wg.Wait()

	fmt.Printf("Loaded %d points from shapefile\n", count.Load())

	// Clear point tile cache
	g.PointTileCache.mu.Lock()
	g.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	g.PointTileCache.lruList = list.New()
	g.PointTileCache.lruMap = make(map[string]*list.Element)
	g.PointTileCache.mu.Unlock()

	g.needRedraw = true
}
