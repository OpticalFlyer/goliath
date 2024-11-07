// map.go
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"container/list"

	"github.com/hajimehoshi/ebiten/v2"
)

// Constants for different basemaps
const (
	GOOGLEHYBRID = "GOOGLEHYBRID"
	GOOGLEAERIAL = "GOOGLEAERIAL"
	BINGHYBRID   = "BINGHYBRID"
	BINGAERIAL   = "BINGAERIAL"
	OSM          = "OSM"
)

// Define maximum zoom levels for each basemap
var maxZoomLevels = map[string]int{
	GOOGLEHYBRID: 21,
	GOOGLEAERIAL: 21,
	BINGHYBRID:   20,
	BINGAERIAL:   20,
	OSM:          19,
}

// TileImageCache manages cached tiles using a nested map for thread-safe access and LRU eviction
type TileImageCache struct {
	cache    map[int]map[int]map[int]*ebiten.Image
	requests map[int]map[int]map[int]bool // Tracks in-progress requests

	// LRU components
	lruList  *list.List               // List of tile keys in LRU order
	lruMap   map[string]*list.Element // Map from tile key to list element
	maxTiles int                      // Maximum number of tiles to cache

	mu sync.Mutex
}

// NewTileImageCache initializes a new TileImageCache with LRU and returns a pointer to it
func NewTileImageCache(maxTiles int) *TileImageCache {
	return &TileImageCache{
		cache:    make(map[int]map[int]map[int]*ebiten.Image),
		requests: make(map[int]map[int]map[int]bool),
		lruList:  list.New(),
		lruMap:   make(map[string]*list.Element),
		maxTiles: maxTiles,
	}
}

// ClearCache clears all cached tiles and resets state for new tile requests.
func (cache *TileImageCache) ClearCache() {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Clear cache data
	cache.cache = make(map[int]map[int]map[int]*ebiten.Image)
	cache.requests = make(map[int]map[int]map[int]bool)

	// Reset LRU list
	cache.lruList.Init()
	cache.lruMap = make(map[string]*list.Element)
}

// ClearDownloadQueue clears any pending downloads in the queue and resets the requested marks.
func ClearDownloadQueue(tileCache *TileImageCache) {
	// Draining the channel by reading all pending requests
	for len(downloadQueue) > 0 {
		req := <-downloadQueue
		// Unmark the tile as requested
		tileCache.UnmarkRequested(req.zoom, req.tileX, req.tileY)
	}
}

// Set caches an image for a specific tile and manages LRU eviction
func (cache *TileImageCache) Set(zoom, x, y int, img *ebiten.Image) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if _, ok := cache.cache[zoom]; !ok {
		cache.cache[zoom] = make(map[int]map[int]*ebiten.Image)
	}
	if _, ok := cache.cache[zoom][x]; !ok {
		cache.cache[zoom][x] = make(map[int]*ebiten.Image)
	}
	cache.cache[zoom][x][y] = img

	// Update LRU
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)
	if elem, exists := cache.lruMap[key]; exists {
		cache.lruList.MoveToFront(elem)
	} else {
		elem := cache.lruList.PushFront(key)
		cache.lruMap[key] = elem

		// Evict tiles if necessary
		if cache.lruList.Len() > cache.maxTiles {
			oldest := cache.lruList.Back()
			if oldest != nil {
				oldestKey := oldest.Value.(string)
				cache.lruList.Remove(oldest)
				delete(cache.lruMap, oldestKey)

				// Parse the key to zoom, x, y
				var oz, ox, oy int
				fmt.Sscanf(oldestKey, "%d-%d-%d", &oz, &ox, &oy)
				delete(cache.cache[oz][ox], oy)
				if len(cache.cache[oz][ox]) == 0 {
					delete(cache.cache[oz], ox)
				}
				if len(cache.cache[oz]) == 0 {
					delete(cache.cache, oz)
				}
			}
		}
	}
}

// Get retrieves a cached image for a specific tile and updates LRU
func (cache *TileImageCache) Get(zoom, x, y int) (*ebiten.Image, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if _, ok := cache.cache[zoom]; !ok {
		return nil, false
	}
	if _, ok := cache.cache[zoom][x]; !ok {
		return nil, false
	}
	img, ok := cache.cache[zoom][x][y]
	if !ok {
		return nil, false
	}

	// Update LRU
	key := fmt.Sprintf("%d-%d-%d", zoom, x, y)
	if elem, exists := cache.lruMap[key]; exists {
		cache.lruList.MoveToFront(elem)
	}

	return img, true
}

// CountTiles returns the total number of tiles currently cached
func (cache *TileImageCache) CountTiles() int {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	return cache.lruList.Len()
}

// EstimateCacheSizeGB estimates the total size of the cached tiles in gigabytes
func (cache *TileImageCache) EstimateCacheSizeGB() float64 {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	tileCount := cache.lruList.Len()

	// Each tile is approximately 0.25 MB
	totalMB := float64(tileCount) * 0.25
	totalGB := totalMB / 1024.0
	return totalGB
}

// IsRequested checks if a tile is currently being requested
func (cache *TileImageCache) IsRequested(zoom, x, y int) bool {
	if _, ok := cache.requests[zoom]; !ok {
		return false
	}
	if _, ok := cache.requests[zoom][x]; !ok {
		return false
	}
	return cache.requests[zoom][x][y]
}

// MarkRequested marks a tile as being requested
func (cache *TileImageCache) MarkRequested(zoom, x, y int) {
	if _, ok := cache.requests[zoom]; !ok {
		cache.requests[zoom] = make(map[int]map[int]bool)
	}
	if _, ok := cache.requests[zoom][x]; !ok {
		cache.requests[zoom][x] = make(map[int]bool)
	}
	cache.requests[zoom][x][y] = true
}

// UnmarkRequested removes the request mark from a tile
func (cache *TileImageCache) UnmarkRequested(zoom, x, y int) {
	if _, ok := cache.requests[zoom]; !ok {
		return
	}
	if _, ok := cache.requests[zoom][x]; !ok {
		return
	}
	delete(cache.requests[zoom][x], y)
	if len(cache.requests[zoom][x]) == 0 {
		delete(cache.requests[zoom], x)
	}
	if len(cache.requests[zoom]) == 0 {
		delete(cache.requests, zoom)
	}
}

// DownloadRequest represents a request to download a specific tile
type DownloadRequest struct {
	tileCache *TileImageCache
	zoom      int
	tileX     int
	tileY     int
	basemap   string
}

// downloadQueue is a buffered channel for tile download requests
var downloadQueue = make(chan DownloadRequest, 1000) // Increased buffer size to 1000

// tileLoadedChan is used to notify when a tile has been loaded
var tileLoadedChan = make(chan struct{}, 100)

// startWorkerPool initializes a pool of workers to handle tile downloads
func startWorkerPool(numWorkers int, tileCache *TileImageCache) {
	for i := 0; i < numWorkers; i++ {
		go tileDownloader(tileCache)
	}
}

// tileDownloader is a worker that processes tile download requests
func tileDownloader(tileCache *TileImageCache) {
	for req := range downloadQueue {
		img, err := downloadTileImage(req.tileX, req.tileY, req.zoom, req.basemap)
		if err == nil {
			tileCache.Set(req.zoom, req.tileX, req.tileY, img)
			// Unmark the tile as requested
			tileCache.UnmarkRequested(req.zoom, req.tileX, req.tileY)
			// Notify that a tile has been loaded
			tileLoadedChan <- struct{}{}
		} else {
			fmt.Printf("Error downloading tile (%d, %d, %d): %v\n", req.zoom, req.tileX, req.tileY, err)
			// Unmark the tile even on failure to allow retry
			tileCache.UnmarkRequested(req.zoom, req.tileX, req.tileY)
		}
	}
}

// drawTile attempts to draw a tile from the cache or requests its download
func drawTile(screen *ebiten.Image, emptyTile *ebiten.Image, tileCache *TileImageCache, tileX, tileY, zoom int, basemap string, op *ebiten.DrawImageOptions) bool {
	maxTile := int(math.Pow(2, float64(zoom)))

	// Clamp tileX and tileY to valid ranges
	if tileX < 0 || tileX >= maxTile || tileY < 0 || tileY >= maxTile {
		screen.DrawImage(emptyTile, op)
		return true
	}

	cachedImg, ok := tileCache.Get(zoom, tileX, tileY)
	if ok {
		screen.DrawImage(cachedImg, op)
		return false
	} else {
		// Draw the empty tile
		screen.DrawImage(emptyTile, op)

		// Check if the tile is already being requested
		if !tileCache.IsRequested(zoom, tileX, tileY) {
			tileCache.MarkRequested(zoom, tileX, tileY)
			// Add a download request to the queue without blocking
			select {
			case downloadQueue <- DownloadRequest{
				tileCache: tileCache,
				zoom:      zoom,
				tileX:     tileX,
				tileY:     tileY,
				basemap:   basemap,
			}:
			default:
				// Queue is full; optionally log or handle this case
				fmt.Println("Download queue is full. Skipping tile request:", zoom, tileX, tileY)
			}
		}
		return true
	}
}

// buildTilePath constructs the file path for caching a tile
func buildTilePath(basemap string, zoom, x, y int) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	var fileExtension string
	if basemap == OSM {
		fileExtension = "png"
	} else {
		fileExtension = "jpg"
	}
	tilePath := filepath.Join(homeDir, ".tilecache", basemap, fmt.Sprintf("%d-%d-%d.%s", zoom, x, y, fileExtension))
	return tilePath, nil
}

// saveTileToDisk saves the downloaded tile data to disk
func saveTileToDisk(tilePath string, data []byte) error {
	// Ensure the directory exists
	dir := filepath.Dir(tilePath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	// Write the file
	err := os.WriteFile(tilePath, data, 0644)
	if err != nil {
		return err
	}

	return nil
}

// downloadTileImage downloads a tile image from the appropriate source
func downloadTileImage(x, y, zoom int, basemap string) (*ebiten.Image, error) {
	tilePath, err := buildTilePath(basemap, zoom, x, y)
	if err != nil {
		fmt.Printf("Failed to build tile path: %s\n", err)
	}

	// Check if tile exists on disk
	if _, err := os.Stat(tilePath); err == nil {
		// Tile exists, load from disk
		fileData, err := os.ReadFile(tilePath)
		if err != nil {
			return nil, err
		}
		// Decode image based on basemap
		var img image.Image
		if basemap == OSM {
			img, err = png.Decode(bytes.NewReader(fileData))
		} else {
			img, err = jpeg.Decode(bytes.NewReader(fileData))
		}
		if err != nil {
			return nil, err
		}
		return ebiten.NewImageFromImage(img), nil
	}

	// Build URL based on basemap
	var url string
	if basemap == BINGHYBRID {
		q := getQuadKey(zoom, x, y)
		url = fmt.Sprintf("http://ecn.t1.tiles.virtualearth.net/tiles/h%s.jpeg?g=129&mkt=en-US&shading=hill&stl=H", q)
	} else if basemap == BINGAERIAL {
		q := getQuadKey(zoom, x, y)
		url = fmt.Sprintf("http://ecn.t1.tiles.virtualearth.net/tiles/a%s.jpeg?g=129&mkt=en-US&shading=hill&stl=H", q)
	} else if basemap == GOOGLEAERIAL {
		url = fmt.Sprintf("https://mt1.google.com/vt/lyrs=s&x=%d&y=%d&z=%d", x, y, zoom)
	} else if basemap == GOOGLEHYBRID {
		url = fmt.Sprintf("https://mt1.google.com/vt/lyrs=s,h&x=%d&y=%d&z=%d", x, y, zoom)
	} else {
		url = fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", zoom, x, y)
	}

	// Download the tile
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "TileMapViewer/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to download tile: %s\n", resp.Status)
		return nil, fmt.Errorf("failed to download tile: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decode image
	var img image.Image
	if basemap == OSM {
		img, err = png.Decode(bytes.NewReader(data))
	} else {
		img, err = jpeg.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, err
	}

	// Save tile to disk
	if err := saveTileToDisk(tilePath, data); err != nil {
		fmt.Println("Failed to save tile to disk:", err)
	}

	return ebiten.NewImageFromImage(img), nil
}

// getQuadKey generates a quadkey for Bing maps based on tile coordinates
func getQuadKey(zoom, tileX, tileY int) string {
	var quadKey string
	for i := zoom; i > 0; i-- {
		var digit int
		mask := 1 << (i - 1)
		if (tileX & mask) != 0 {
			digit += 1
		}
		if (tileY & mask) != 0 {
			digit += 2
		}
		quadKey += fmt.Sprintf("%d", digit)
	}
	return quadKey
}
