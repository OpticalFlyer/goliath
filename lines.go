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

func (g *Game) getLineTile(tileX, tileY, zoom int) *LineTile {
	g.LineTileCache.mu.RLock()
	if zoomLevel, exists := g.LineTileCache.cache[zoom]; exists {
		if xLevel, exists := zoomLevel[tileX]; exists {
			if tile, exists := xLevel[tileY]; exists {
				g.LineTileCache.mu.RUnlock()
				return tile
			}
		}
	}
	g.LineTileCache.mu.RUnlock()

	// Create new tile
	tile := g.renderLineTile(tileX, tileY, zoom)
	if tile == nil {
		return nil
	}

	// Cache the tile
	g.LineTileCache.mu.Lock()
	defer g.LineTileCache.mu.Unlock()

	if _, exists := g.LineTileCache.cache[zoom]; !exists {
		g.LineTileCache.cache[zoom] = make(map[int]map[int]*LineTile)
	}
	if _, exists := g.LineTileCache.cache[zoom][tileX]; !exists {
		g.LineTileCache.cache[zoom][tileX] = make(map[int]*LineTile)
	}
	g.LineTileCache.cache[zoom][tileX][tileY] = tile

	return tile
}

// renderLineTile renders lines within a tile
func (g *Game) renderLineTile(tileX, tileY, zoom int) *LineTile {
	bounds := getLineTileBounds(tileX, tileY, zoom)
	tile := &LineTile{
		Image:     ebiten.NewImage(tileSizePixels, tileSizePixels),
		Bounds:    bounds,
		ZoomLevel: zoom,
	}

	lines := g.PolylineLayer.Index.Search(bounds)
	tileOriginX := float64(tileX * tileSizePixels)
	tileOriginY := float64(tileY * tileSizePixels)

	for _, l := range lines {
		line := l.(*LineString)
		if len(line.Points) < 2 {
			continue
		}

		// Draw line segments directly
		for i := 0; i < len(line.Points)-1; i++ {
			x1, y1 := latLngToPixel(line.Points[i].Lat, line.Points[i].Lon, zoom)
			x2, y2 := latLngToPixel(line.Points[i+1].Lat, line.Points[i+1].Lon, zoom)

			// Convert to tile coordinates
			x1 -= tileOriginX
			y1 -= tileOriginY
			x2 -= tileOriginX
			y2 -= tileOriginY

			vector.StrokeLine(tile.Image, float32(x1), float32(y1), float32(x2), float32(y2), float32(lineWidth), color.RGBA{0, 0, 255, 255}, false)
		}
	}

	return tile
}

// DrawLines renders visible line tiles
func (g *Game) DrawLines(screen *ebiten.Image) {
	if !g.PolylineLayer.Visible {
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

			tile := g.getLineTile(tileX, tileY, g.zoom)
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

func (g *Game) InitializeTestLines(numLines int) {
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
				// Random starting point in continental US
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
		g.PolylineLayer.Index.Insert(line, line.Bounds())
		count++
		if count%100 == 0 {
			fmt.Printf("Generated %d lines...\n", count)
		}
	}

	fmt.Printf("Added %d lines to R-tree\n", count)
}
