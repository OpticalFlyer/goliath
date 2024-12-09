// main.go
package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Goliath struct encapsulates the program state and behavior
type Goliath struct {
	ScreenWidth    int
	ScreenHeight   int
	basemap        string
	centerLat      float64
	centerLon      float64
	zoom           int
	tileCache      *TileImageCache
	emptyTile      *ebiten.Image
	offscreenImage *ebiten.Image
	needRedraw     bool

	// Fields for mouse drag panning
	isDragging      bool
	dragStartX      int
	dragStartY      int
	dragStartPixelX float64
	dragStartPixelY float64

	lastZoom       int
	lastZoomChange int64 // Frame counter when zoom last changed
	frameCount     int64
}

// Initialize sets up the initial state of the program
func Initialize() (*Goliath, error) {
	// Initialize the cache with a maximum of 10000 tiles
	tileCache := NewTileImageCache(10000)

	g := &Goliath{
		centerLat:    39.8283, // Center of the US
		centerLon:    -98.5795,
		zoom:         5,            // Default zoom level
		basemap:      GOOGLEAERIAL, // Default basemap
		ScreenWidth:  1024,
		ScreenHeight: 768,
		tileCache:    tileCache, // tileCache is *TileImageCache
		needRedraw:   true,
	}

	// Initialize an empty tile (solid color) for missing tiles
	g.emptyTile = ebiten.NewImage(256, 256)
	solidColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	g.emptyTile.Fill(solidColor)

	g.offscreenImage = ebiten.NewImage(g.ScreenWidth, g.ScreenHeight)

	return g, nil
}

// Update handles the program state updates, including panning and zooming
func (g *Goliath) Update() error {
	// Handle tile loaded notifications
	select {
	case <-tileLoadedChan:
		g.needRedraw = true
	default:
		// No tile loaded this frame
	}

	// Handle panning with middle mouse button
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		mouseX, mouseY := ebiten.CursorPosition()
		if !g.isDragging {
			// Start dragging
			g.isDragging = true
			g.dragStartX = mouseX
			g.dragStartY = mouseY
			g.dragStartPixelX, g.dragStartPixelY = latLngToPixel(g.centerLat, g.centerLon, g.zoom)
		} else {
			// Continue dragging
			deltaX := mouseX - g.dragStartX
			deltaY := mouseY - g.dragStartY

			newCenterPixelX := g.dragStartPixelX - float64(deltaX)
			newCenterPixelY := g.dragStartPixelY - float64(deltaY)

			newLat, newLon := pixelToLatLng(newCenterPixelX, newCenterPixelY, g.zoom)

			g.centerLat = math.Min(math.Max(newLat, -85.05112878), 85.05112878)
			g.centerLon = math.Min(math.Max(newLon, -180), 180)

			g.needRedraw = true
		}
	} else {
		g.isDragging = false
	}

	// Handle panning with arrow keys
	panSpeed := 100.0 // Adjust the panning speed as needed
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.centerLat += panSpeed / math.Pow(2, float64(g.zoom))
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.centerLat -= panSpeed / math.Pow(2, float64(g.zoom))
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.centerLon -= panSpeed / math.Pow(2, float64(g.zoom))
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.centerLon += panSpeed / math.Pow(2, float64(g.zoom))
		g.needRedraw = true
	}

	// Zoom handling with mouse wheel
	g.frameCount++
	if _, scrollY := ebiten.Wheel(); scrollY != 0 {
		// Record when zoom changed
		if g.zoom != g.lastZoom {
			g.lastZoomChange = g.frameCount
			g.lastZoom = g.zoom
		}

		mouseX, mouseY := ebiten.CursorPosition()

		// Adjust zoom level
		if scrollY > 0 && g.zoom < maxZoomLevels[g.basemap] {
			// Store pre-zoom world coordinates
			preZoomLat, preZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

			g.zoom++

			// Get post-zoom coordinates and adjust center to maintain mouse position
			postZoomLat, postZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
			g.centerLat += preZoomLat - postZoomLat
			g.centerLon += preZoomLon - postZoomLon
		} else if scrollY < 0 && g.zoom > minZoom {
			// Store pre-zoom world coordinates
			preZoomLat, preZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

			g.zoom--

			// Get post-zoom coordinates and adjust center to maintain mouse position
			postZoomLat, postZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
			g.centerLat += preZoomLat - postZoomLat
			g.centerLon += preZoomLon - postZoomLon
		}

		// Clear the download queue and reset requested marks when zoom level changes
		if g.zoom != g.lastZoom {
			ClearDownloadQueue(g.tileCache)
			g.lastZoomChange = g.frameCount
			g.lastZoom = g.zoom
		}

		// Clamp coordinates to valid ranges after zoom
		g.centerLat = math.Min(math.Max(g.centerLat, -85.05112878), 85.05112878)
		g.centerLon = math.Min(math.Max(g.centerLon, -180), 180)

		g.needRedraw = true
	}

	return nil
}

// Draw renders the current program state to the screen
func (g *Goliath) Draw(screen *ebiten.Image) {
	if g.needRedraw {
		drawBasemapTiles(g)
	}

	// Draw the tile map
	screen.DrawImage(g.offscreenImage, nil)

	// Gather debug information
	mouseX, mouseY := ebiten.CursorPosition()
	lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

	// Fetch additional debug data
	tilesCached := g.tileCache.CountTiles()
	cacheSizeGB := g.tileCache.EstimateCacheSizeGB()
	tilesInQueue := len(downloadQueue)

	// Create the debug string with additional information
	debugString := fmt.Sprintf(
		"Zoom: %d\nCenter: %.6f, %.6f\nBasemap: %s\nFPS: %.0f\nMouse: (%d, %d)\nLat: %.6f, Lon: %.6f\nTiles Cached: %d\nCache Size: %.4f GB\nTiles in Queue: %d",
		g.zoom,
		g.centerLat,
		g.centerLon,
		g.basemap,
		ebiten.ActualFPS(),
		mouseX,
		mouseY,
		lat,
		lon,
		tilesCached,
		cacheSizeGB,
		tilesInQueue,
	)

	// Draw semi-transparent background for debug text
	debugBg := ebiten.NewImage(200, 200)
	debugBg.Fill(color.RGBA{0, 0, 0, 128})
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(debugBg, op)

	// Display the debug information
	ebitenutil.DebugPrint(screen, debugString)
}

// Layout defines the screen dimensions
func (g *Goliath) Layout(outsideWidth, outsideHeight int) (int, int) {
	if g.ScreenWidth != outsideWidth || g.ScreenHeight != outsideHeight {
		g.offscreenImage = ebiten.NewImage(outsideWidth, outsideHeight)
		g.needRedraw = true
	}

	g.ScreenWidth = outsideWidth
	g.ScreenHeight = outsideHeight
	return outsideWidth, outsideHeight
}

func main() {
	// Output the current working directory to the terminal
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current working directory: %v", err)
	}
	fmt.Printf("Current working directory: %s\n", wd)

	g, err := Initialize()
	if err != nil {
		log.Fatalf("Initialization error: %v", err)
	}

	// Start the worker pool for tile downloading with 10 workers
	startWorkerPool(10, g.tileCache)

	ebiten.SetWindowSize(g.ScreenWidth, g.ScreenHeight)
	ebiten.SetWindowTitle("Goliath")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetCursorMode(ebiten.CursorModeVisible)

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
