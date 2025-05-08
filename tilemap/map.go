package tilemap

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/http"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	// TileSize is the size of map tiles in pixels
	TileSize = 256
	// MaxZoomLevel is the maximum zoom level supported
	MaxZoomLevel = 19
)

// TileRange defines the range of tiles needed to cover the viewport
type TileRange struct {
	MinX, MaxX int
	MinY, MaxY int
}

// TileKey uniquely identifies a map tile
type TileKey struct {
	Zoom int
	X    int
	Y    int
}

// TileMap manages the slippy map tile system
type TileMap struct {
	// View state
	CenterLat    float64
	CenterLon    float64
	Zoom         int
	ScreenWidth  int
	ScreenHeight int

	// Tile management
	tileCache       map[TileKey]*ebiten.Image
	cacheMu         sync.RWMutex
	placeholderTile *ebiten.Image
	fetching        map[TileKey]bool
	fetchingMu      sync.Mutex
}

// New creates a new TileMap instance
func New(screenWidth, screenHeight int, lat, lon float64, zoom int) *TileMap {
	placeholder := ebiten.NewImage(TileSize, TileSize)
	placeholder.Fill(color.Black) // Black placeholder

	return &TileMap{
		ScreenWidth:     screenWidth,
		ScreenHeight:    screenHeight,
		CenterLat:       lat,
		CenterLon:       lon,
		Zoom:            zoom,
		tileCache:       make(map[TileKey]*ebiten.Image),
		placeholderTile: placeholder,
		fetching:        make(map[TileKey]bool),
	}
}

// CalculateVisibleTileRange determines which tiles are needed for the current view
func (tm *TileMap) CalculateVisibleTileRange() (TileRange, float64, float64) {
	centerXTileF, centerYTileF := LatLonToTileFloat(tm.CenterLat, tm.CenterLon, tm.Zoom)

	topLeftXTileF := centerXTileF - float64(tm.ScreenWidth)/2.0/TileSize
	topLeftYTileF := centerYTileF - float64(tm.ScreenHeight)/2.0/TileSize
	bottomRightXTileF := centerXTileF + float64(tm.ScreenWidth)/2.0/TileSize
	bottomRightYTileF := centerYTileF + float64(tm.ScreenHeight)/2.0/TileSize

	minTileX := int(math.Floor(topLeftXTileF))
	minTileY := int(math.Floor(topLeftYTileF))
	maxTileX := int(math.Floor(bottomRightXTileF))
	maxTileY := int(math.Floor(bottomRightYTileF))

	maxCoord := 1 << tm.Zoom
	return TileRange{
		MinX: max(0, minTileX),
		MaxX: min(maxCoord-1, maxTileX),
		MinY: max(0, minTileY),
		MaxY: min(maxCoord-1, maxTileY),
	}, centerXTileF, centerYTileF
}

// Draw renders the visible tiles to the screen
func (tm *TileMap) Draw(screen *ebiten.Image, debugMode bool) TileRange {
	tileRange, centerXTileF, centerYTileF := tm.CalculateVisibleTileRange()

	tm.cacheMu.RLock()
	tm.fetchingMu.Lock()
	defer tm.fetchingMu.Unlock()
	defer tm.cacheMu.RUnlock()

	// Debug colors
	redColor := color.RGBA{R: 255, A: 255}
	strokeWidth := float32(1.0)

	// Iterate through the required tile grid
	for ty := tileRange.MinY; ty <= tileRange.MaxY; ty++ {
		for tx := tileRange.MinX; tx <= tileRange.MaxX; tx++ {
			key := TileKey{Zoom: tm.Zoom, X: tx, Y: ty}
			tileImg, found := tm.tileCache[key]
			isFetching := tm.fetching[key]

			drawX := float64(tm.ScreenWidth)/2 - (centerXTileF-float64(tx))*TileSize
			drawY := float64(tm.ScreenHeight)/2 - (centerYTileF-float64(ty))*TileSize
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(drawX, drawY)

			if !found && !isFetching {
				tm.fetching[key] = true
				go tm.fetchAndCacheTile(key)
			}

			if found && tileImg != nil {
				screen.DrawImage(tileImg, op)
				if debugMode {
					// Draw blue tint and grid for loaded tiles
					vector.DrawFilledRect(screen, float32(drawX), float32(drawY),
						float32(TileSize), float32(TileSize),
						color.RGBA{B: 100, A: 100}, false)
					vector.StrokeRect(screen, float32(drawX), float32(drawY),
						float32(TileSize), float32(TileSize),
						strokeWidth, redColor, false)
					ebitenutil.DebugPrintAt(screen,
						fmt.Sprintf("%d/%d/%d", tm.Zoom, tx, ty),
						int(drawX)+2, int(drawY)+2)
				}
			} else {
				if tm.placeholderTile != nil {
					screen.DrawImage(tm.placeholderTile, op)
				}
				if debugMode {
					// Draw yellow or red tint for loading/needed tiles
					fillColor := color.RGBA{R: 50, A: 50} // Red tint for needed
					if isFetching {
						fillColor = color.RGBA{R: 100, G: 100, B: 0, A: 50} // Yellow tint for fetching
					}
					vector.DrawFilledRect(screen, float32(drawX), float32(drawY),
						float32(TileSize), float32(TileSize),
						fillColor, false)
					vector.StrokeRect(screen, float32(drawX), float32(drawY),
						float32(TileSize), float32(TileSize),
						strokeWidth, redColor, false)
					status := "Needed"
					if isFetching {
						status = "Fetching"
					}
					ebitenutil.DebugPrintAt(screen,
						fmt.Sprintf("%s: %d/%d/%d", status, tm.Zoom, tx, ty),
						int(drawX)+2, int(drawY)+2)
				}
			}
		}
	}

	return tileRange
}

// fetchAndCacheTile fetches and caches a single tile
func (tm *TileMap) fetchAndCacheTile(key TileKey) {
	defer func() {
		tm.fetchingMu.Lock()
		delete(tm.fetching, key)
		tm.fetchingMu.Unlock()
	}()

	tileImg, err := fetchTile(key.Zoom, key.X, key.Y)
	if err != nil {
		log.Printf("Error fetching tile %d/%d/%d: %v", key.Zoom, key.X, key.Y, err)
		return
	}

	tm.cacheMu.Lock()
	tm.tileCache[key] = tileImg
	tm.cacheMu.Unlock()
}

// LatLonToTileFloat converts WGS84 coordinates to fractional tile coordinates
func LatLonToTileFloat(lat, lon float64, zoom int) (x, y float64) {
	latRad := lat * math.Pi / 180.0
	n := math.Pow(2.0, float64(zoom))
	x = (lon + 180.0) / 360.0 * n
	latRad = math.Max(math.Min(latRad, 1.48442), -1.48442)
	y = (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n
	return x, y
}

// Helper functions
func fetchTile(zoom, x, y int) (*ebiten.Image, error) {
	maxCoord := 1 << zoom
	if x < 0 || x >= maxCoord || y < 0 || y >= maxCoord {
		return nil, fmt.Errorf("tile coordinates (%d, %d) out of range for zoom %d", x, y, zoom)
	}

	tileURL := fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", zoom, x, y)
	client := &http.Client{}
	req, err := http.NewRequest("GET", tileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s failed: %w", tileURL, err)
	}
	req.Header.Set("User-Agent", "FiberForge 1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s failed: %w", tileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch tile %s: %s", tileURL, resp.Status)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decoding image %s failed: %w", tileURL, err)
	}

	return ebiten.NewImageFromImage(img), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
