// points.go
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

const (
	pointSpriteSize = 13
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
	pointSprite         *ebiten.Image
	selectedPointSprite *ebiten.Image
)

func init() {
	// Create normal point sprite
	pointSprite = ebiten.NewImage(pointSpriteSize, pointSpriteSize)
	vector.DrawFilledCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, color.White, false)
	vector.StrokeCircle(pointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, 1, color.Black, false)

	// Create selected point sprite
	selectedPointSprite = ebiten.NewImage(pointSpriteSize, pointSpriteSize)
	// Yellow fill with thicker black border
	vector.DrawFilledCircle(selectedPointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, color.RGBA{255, 255, 0, 255}, false)
	vector.StrokeCircle(selectedPointSprite, pointSpriteSize/2, pointSpriteSize/2, (pointSpriteSize-2)/2, 2, color.Black, false)
}

// InitializeTestPoints adds random points in parallel using worker pools
func (g *Game) InitializeTestPoints(layer *Layer, numPoints int) {
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
		layer.PointLayer.Index.Insert(point, point.Bounds())
		count++
		if count%100000 == 0 {
			fmt.Printf("Generated %d points...\n", count)
		}
	}

	fmt.Printf("Added %d points to R-tree\n", layer.PointLayer.Index.Size)

	// Clear entire point tile cache
	layer.PointTileCache.mu.Lock()
	layer.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
	layer.PointTileCache.lruList = list.New()
	layer.PointTileCache.lruMap = make(map[string]*list.Element)
	layer.PointTileCache.mu.Unlock()

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
func (g *Game) renderPointTile(layer *Layer, tileX, tileY, zoom int) *PointTile {
	bounds := getTileBoundsWithPadding(tileX, tileY, zoom)
	tile := &PointTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	points := layer.PointLayer.Index.Search(bounds)
	tileOriginX := float64(tileX * tileSizePixels)
	tileOriginY := float64(tileY * tileSizePixels)

	for _, p := range points {
		point := p.(*Point)
		worldX, worldY := latLngToPixel(point.Lat, point.Lon, zoom)

		if point.IconImage != nil {
			w, h := point.IconImage.Bounds().Dx(), point.IconImage.Bounds().Dy()
			scale := point.Scale
			if scale == 0 {
				scale = 1.0
			}
			scale = 1.0 // Disable scaling for now

			// Calculate position with hotspot offset
			// Adjust X from the left and Y from the bottom
			localX := worldX - tileOriginX - point.HotSpot.X*scale
			localY := worldY - tileOriginY - (float64(h) - point.HotSpot.Y*scale)

			// Check if icon would be visible in tile
			if localX > -float64(w)*scale && localX < float64(tileSizePixels) &&
				localY > -float64(h)*scale && localY < float64(tileSizePixels) {

				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(scale, scale)
				op.GeoM.Translate(localX, localY)

				tile.Image.DrawImage(point.IconImage, op)

				// Draw an "X" at the hotspot location for debugging
				/*const size = 5.0
				clr := color.RGBA{255, 0, 0, 255} // Red color for the "X"

				// Calculate hotspot position
				hotspotX := localX + point.HotSpot.X*scale
				hotspotY := localY + (float64(h) - point.HotSpot.Y*scale)

				// Draw two lines to form an "X"
				vector.StrokeLine(tile.Image, float32(hotspotX-size), float32(hotspotY-size), float32(hotspotX+size), float32(hotspotY+size), 1, clr, false)
				vector.StrokeLine(tile.Image, float32(hotspotX-size), float32(hotspotY+size), float32(hotspotX+size), float32(hotspotY-size), 1, clr, false)
				*/
			}
		} else {
			// Use default sprite
			localX := worldX - tileOriginX - float64(pointSpriteSize)/2
			localY := worldY - tileOriginY - float64(pointSpriteSize)/2

			if localX > -pointSpriteSize && localX < tileSizePixels &&
				localY > -pointSpriteSize && localY < tileSizePixels {

				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(localX, localY)

				// Use different sprite based on selection state
				if point.Selected {
					tile.Image.DrawImage(selectedPointSprite, op)
				} else {
					tile.Image.DrawImage(pointSprite, op)
				}
			}
		}
	}

	return tile
}

// Modified DrawPoints to use tile cache
func (g *Game) DrawPoints(screen *ebiten.Image) {
	for _, rootLayer := range g.layers {
		WalkLayers(rootLayer, func(layer *Layer) {
			if !layer.IsEffectivelyVisible() {
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

					tile := g.getPointTile(layer, tileX, tileY, g.zoom)
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

// Get or create point tile
func (g *Game) getPointTile(layer *Layer, tileX, tileY, zoom int) *PointTile {
	// First check cache with read lock
	layer.PointTileCache.mu.RLock()
	if tile := layer.PointTileCache.get(zoom, tileX, tileY); tile != nil {
		layer.PointTileCache.mu.RUnlock()
		return tile
	}
	layer.PointTileCache.mu.RUnlock()

	// Create new tile without holding any locks
	tile := g.renderPointTile(layer, tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Try to store in cache with write lock
	layer.PointTileCache.mu.Lock()
	layer.PointTileCache.set(zoom, tileX, tileY, tile)
	layer.PointTileCache.mu.Unlock()

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
func (g *Game) clearAffectedTiles(layer *Layer, point *Point) {
	layer.PointTileCache.mu.Lock()
	defer layer.PointTileCache.mu.Unlock()

	// For each zoom level in the cache
	for zoom := range layer.PointTileCache.cache {
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
				if xLevel, exists := layer.PointTileCache.cache[zoom][tileX]; exists {
					if _, exists := xLevel[tileY]; exists {
						// Remove from LRU
						key := fmt.Sprintf("%d-%d-%d", zoom, tileX, tileY)
						if element, exists := layer.PointTileCache.lruMap[key]; exists {
							layer.PointTileCache.lruList.Remove(element)
							delete(layer.PointTileCache.lruMap, key)
						}
						// Remove tile
						delete(xLevel, tileY)
					}
				}
			}
		}
	}
	g.needRedraw = true
}
