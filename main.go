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
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Game struct encapsulates the game state and behavior
type Game struct {
	ScreenWidth    int
	ScreenHeight   int
	basemap        string
	TextBoxText    string
	LastCmdText    string
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

	// Geometry layers
	PointLayer    *GeometryLayer
	PolylineLayer *GeometryLayer
	PolygonLayer  *GeometryLayer

	PointTileCache *PointTileCache

	insertMode  bool   // Track if we're in point insertion mode
	lastCommand string // Store the last successful command
}

// GeometryLayer represents a layer of geometries with spatial indexing
type GeometryLayer struct {
	Name    string
	Index   *RTree
	Visible bool
	buffer  *ebiten.Image // Offscreen buffer
}

// Initialize sets up the initial state of the game
func Initialize() (*Game, error) {
	// Initialize the cache with a maximum of 10000 tiles
	tileCache := NewTileImageCache(10000)

	g := &Game{
		centerLat:      39.8283, // Center of the US
		centerLon:      -98.5795,
		zoom:           5,            // Default zoom level
		basemap:        GOOGLEAERIAL, // Default basemap
		ScreenWidth:    1024,
		ScreenHeight:   768,
		tileCache:      tileCache, // tileCache is *TileImageCache
		needRedraw:     true,
		PointTileCache: NewPointTileCache(1000),
	}

	// Initialize an empty tile (solid color) for missing tiles
	g.emptyTile = ebiten.NewImage(256, 256)
	solidColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	g.emptyTile.Fill(solidColor)

	g.offscreenImage = ebiten.NewImage(g.ScreenWidth, g.ScreenHeight)

	g.PointLayer = &GeometryLayer{
		Name:    "Points",
		Index:   NewRTree(),
		Visible: true,
		buffer:  ebiten.NewImage(g.ScreenWidth, g.ScreenHeight),
	}

	g.PolylineLayer = &GeometryLayer{
		Name:    "Polylines",
		Index:   NewRTree(),
		Visible: true,
	}

	g.PolygonLayer = &GeometryLayer{
		Name:    "Polygons",
		Index:   NewRTree(),
		Visible: true,
	}

	// Initialize test points
	//g.InitializeTestPoints()

	return g, nil
}

// Update handles the game state updates, including panning, zooming, and UI interactions
func (g *Game) Update() error {
	// Handle tile loaded notifications
	select {
	case <-tileLoadedChan:
		g.needRedraw = true
	default:
		// No tile loaded this frame
	}

	// Handle UI input
	g.handleTextInput()

	// Check if Enter or Space was pressed to execute command
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.executeCommand()
		g.TextBoxText = "" // Clear the textbox after executing
	}

	// Handle mouse drag panning
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		mouseX, mouseY := ebiten.CursorPosition()
		if !g.isDragging {
			// Start dragging
			g.isDragging = true
			g.dragStartX = mouseX
			g.dragStartY = mouseY

			// Convert center lat/lon to pixel coordinates
			g.dragStartPixelX, g.dragStartPixelY = latLngToPixel(g.centerLat, g.centerLon, g.zoom)
		} else {
			// Continue dragging
			deltaX := mouseX - g.dragStartX
			deltaY := mouseY - g.dragStartY

			// Calculate new center pixel coordinates by subtracting delta
			newCenterPixelX := g.dragStartPixelX - float64(deltaX)
			newCenterPixelY := g.dragStartPixelY - float64(deltaY)

			// Convert new center pixel coordinates back to lat/lon
			newLat, newLon := pixelToLatLng(newCenterPixelX, newCenterPixelY, g.zoom)

			// Update center coordinates
			g.centerLat = newLat
			g.centerLon = newLon

			// Clamp coordinates to valid ranges
			g.centerLat = math.Min(math.Max(g.centerLat, -85.05112878), 85.05112878)
			g.centerLon = math.Min(math.Max(g.centerLon, -180), 180)

			g.needRedraw = true
		}
	} else {
		// Stop dragging
		g.isDragging = false
	}

	// Zoom handling with mouse wheel
	if _, scrollY := ebiten.Wheel(); scrollY != 0 {
		mouseX, mouseY := ebiten.CursorPosition()
		// Get the world coordinates before zoom
		preZoomLat, preZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

		// Adjust zoom level
		if scrollY > 0 {
			g.zoom++
		} else {
			g.zoom--
		}

		// Limit the zoom level to prevent excessive zooming
		minZoom := 5
		maxZoom := maxZoomLevels[g.basemap]

		if g.zoom < minZoom {
			g.zoom = minZoom
		} else if g.zoom > maxZoom {
			g.zoom = maxZoom
		}

		// Clear the download queue and reset requested marks when zoom level changes
		ClearDownloadQueue(g.tileCache)

		// Get the world coordinates after zoom
		postZoomLat, postZoomLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

		// Calculate the difference and adjust the center to keep the point under the cursor stationary
		g.centerLat += preZoomLat - postZoomLat
		g.centerLon += preZoomLon - postZoomLon

		// Clamp coordinates to valid ranges
		g.centerLat = math.Min(math.Max(g.centerLat, -85.05112878), 85.05112878)
		g.centerLon = math.Min(math.Max(g.centerLon, -180), 180)

		g.needRedraw = true
	}

	// Panning with arrow keys (optional)
	panSpeed := 180 / math.Pow(2, float64(g.zoom))
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.centerLon -= panSpeed
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.centerLon += panSpeed
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.centerLat += panSpeed
		g.needRedraw = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.centerLat -= panSpeed
		g.needRedraw = true
	}

	if g.insertMode {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			mouseX, mouseY := ebiten.CursorPosition()
			lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
			point := &Point{Lat: lat, Lon: lon}
			g.PointLayer.Index.Insert(point, point.Bounds())
			g.clearAffectedTiles(point) // Clear affected tiles
		}
	}

	return nil
}

// Draw renders the current game state to the screen
func (g *Game) Draw(screen *ebiten.Image) {
	if g.needRedraw {
		g.needRedraw = false
		g.offscreenImage.Clear()

		// Calculate global pixel coordinates of the center
		pixelX, pixelY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

		// Calculate top-left pixel coordinates based on window size
		topLeftX := pixelX - float64(g.ScreenWidth)/2
		topLeftY := pixelY - float64(g.ScreenHeight)/2

		// Calculate starting tile indices
		startTileX := int(math.Floor(topLeftX / 256))
		startTileY := int(math.Floor(topLeftY / 256))

		// Calculate pixel offsets within the first tile
		offsetX := int(math.Mod(topLeftX, 256))
		offsetY := int(math.Mod(topLeftY, 256))
		if topLeftX < 0 {
			offsetX += 256
			startTileX--
		}
		if topLeftY < 0 {
			offsetY += 256
			startTileY--
		}

		// Calculate how many tiles are needed to cover the window
		numHorizontalTiles := int(math.Ceil(float64(g.ScreenWidth)/256)) + 2
		numVerticalTiles := int(math.Ceil(float64(g.ScreenHeight)/256)) + 2

		// Draw tiles
		for i := 0; i < numHorizontalTiles; i++ {
			for j := 0; j < numVerticalTiles; j++ {
				tileX := startTileX + i
				tileY := startTileY + j

				// Clamp tileX to valid range
				maxTile := int(math.Pow(2, float64(g.zoom)))
				if tileX < 0 || tileX >= maxTile {
					// Draw empty tile for out-of-bounds longitude
					op := &ebiten.DrawImageOptions{}
					op.GeoM.Translate(float64(i*256-offsetX), float64(j*256-offsetY))
					g.offscreenImage.DrawImage(g.emptyTile, op)
					continue
				}

				// Clamp tileY to valid range
				if tileY < 0 || tileY >= maxTile {
					// Draw empty tile for out-of-bounds latitude
					op := &ebiten.DrawImageOptions{}
					op.GeoM.Translate(float64(i*256-offsetX), float64(j*256-offsetY))
					g.offscreenImage.DrawImage(g.emptyTile, op)
					continue
				}

				// Retrieve the tile image from cache or request download
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(float64(i*256-offsetX), float64(j*256-offsetY))
				drawTile(g.offscreenImage, g.emptyTile, g.tileCache, tileX, tileY, g.zoom, g.basemap, op)
			}
		}
	}

	// Draw the tile map
	screen.DrawImage(g.offscreenImage, nil)

	// Draw geometry layers
	if g.PointLayer.Visible {
		g.DrawPoints(screen)
	}

	// Draw the command textbox (defined in ui.go)
	g.DrawTextbox(screen, g.ScreenWidth, g.ScreenHeight)

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
	debugBg := ebiten.NewImage(200, 150)
	debugBg.Fill(color.RGBA{0, 0, 0, 128})
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(debugBg, op)

	// Display the debug information
	ebitenutil.DebugPrint(screen, debugString)
}

// Layout defines the screen dimensions
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	if g.ScreenWidth != outsideWidth || g.ScreenHeight != outsideHeight {
		g.offscreenImage = ebiten.NewImage(outsideWidth, outsideHeight)
		g.needRedraw = true
	}

	g.ScreenWidth = outsideWidth
	g.ScreenHeight = outsideHeight
	return outsideWidth, outsideHeight
}

// latLngToPixel converts latitude and longitude to global pixel coordinates at a given zoom level.
func latLngToPixel(lat, lng float64, zoom int) (float64, float64) {
	latRad := lat * math.Pi / 180.0
	n := math.Pow(2, float64(zoom))
	x := (lng + 180.0) / 360.0 * 256.0 * n
	y := (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * 256.0 * n
	return x, y
}

// pixelToLatLng converts global pixel coordinates to latitude and longitude at a given zoom level.
func pixelToLatLng(pixelX, pixelY float64, zoom int) (float64, float64) {
	n := math.Pow(2, float64(zoom))
	lng := (pixelX/(256.0*n))*360.0 - 180.0

	yRatio := (1.0 - (pixelY/(256.0*n))*2.0)
	latRad := math.Atan(math.Sinh(math.Pi * yRatio))
	lat := latRad * 180.0 / math.Pi

	return lat, lng
}

// latLngFromPixel converts screen coordinates to latitude and longitude based on the current game state.
func latLngFromPixel(screenX, screenY float64, game *Game) (float64, float64) {
	pixelX, pixelY := latLngToPixel(game.centerLat, game.centerLon, game.zoom)

	// Calculate top-left pixel coordinates based on window size
	topLeftX := pixelX - float64(game.ScreenWidth)/2
	topLeftY := pixelY - float64(game.ScreenHeight)/2

	// Calculate world pixel coordinates of the cursor
	cursorPixelX := topLeftX + screenX
	cursorPixelY := topLeftY + screenY

	// Convert pixel coordinates back to lat/lng
	lat, lon := pixelToLatLng(cursorPixelX, cursorPixelY, game.zoom)

	return lat, lon
}

func main() {
	// Output the current working directory to the terminal
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current working directory: %v", err)
	}
	fmt.Printf("Current working directory: %s\n", wd)

	game, err := Initialize()
	if err != nil {
		log.Fatalf("Initialization error: %v", err)
	}

	// Start the worker pool for tile downloading with 10 workers
	startWorkerPool(10, game.tileCache)

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("Goliath")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetCursorMode(ebiten.CursorModeVisible)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
