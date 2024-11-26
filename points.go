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

		if !g.defaultRender && point.IconImage != nil {
			w, h := point.IconImage.Bounds().Dx(), point.IconImage.Bounds().Dy()
			scale := point.Scale
			if scale == 0 {
				scale = 1.0
			}
			scale = 1.0 // Disable scaling for now

			// Calculate position with hotspot offset
			// Adjust X from the left and Y from the bottom
			var localX, localY float64
			if point.HotSpot.X == 0 && point.HotSpot.Y == 0 {
				// Center the sprite on the geometry point
				localX = worldX - tileOriginX - float64(w)/2
				localY = worldY - tileOriginY - float64(h)/2
			} else {
				// Adjust X from the left and Y from the bottom
				localX = worldX - tileOriginX - point.HotSpot.X*scale
				localY = worldY - tileOriginY - (float64(h) - point.HotSpot.Y*scale)
			}

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
	// Skip tile creation if zoom is changing rapidly
	if !g.isZoomStable() {
		return nil
	}

	// Get tile with read lock
	layer.PointTileCache.mu.RLock()
	tile := layer.PointTileCache.get(zoom, tileX, tileY)
	if tile != nil {
		layer.PointTileCache.mu.RUnlock()
		return tile
	}
	layer.PointTileCache.mu.RUnlock()

	// Create new tile
	tile = g.renderPointTile(layer, tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Store with proper LRU handling
	layer.PointTileCache.mu.Lock()
	layer.PointTileCache.set(zoom, tileX, tileY, tile)
	layer.PointTileCache.mu.Unlock()

	return tile
}

// get retrieves a tile from the cache
func (c *PointTileCache) get(zoom, x, y int) *PointTile {
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)

	if zoomLevel, exists := c.cache[zoom]; exists {
		if xLevel, exists := zoomLevel[x]; exists {
			if tile, exists := xLevel[y]; exists {
				// Update LRU on access
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
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)

	// Handle eviction before adding new tile
	if c.lruList.Len() >= c.maxTiles {
		oldest := c.lruList.Back()
		if oldest != nil {
			oldKey := oldest.Value.(string)
			var oz, ox, oy int
			fmt.Sscanf(oldKey, "%d-%d-%d", &oz, &ox, &oy)

			// Remove from cache
			delete(c.cache[oz][ox], oy)
			if len(c.cache[oz][ox]) == 0 {
				delete(c.cache[oz], ox)
			}
			if len(c.cache[oz]) == 0 {
				delete(c.cache, oz)
			}

			// Remove from LRU
			c.lruList.Remove(oldest)
			delete(c.lruMap, oldKey)
		}
	}

	// Add to cache
	if _, ok := c.cache[zoom]; !ok {
		c.cache[zoom] = make(map[int]map[int]*PointTile)
	}
	if _, ok := c.cache[zoom][x]; !ok {
		c.cache[zoom][x] = make(map[int]*PointTile)
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
