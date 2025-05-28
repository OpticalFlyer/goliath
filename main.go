package main

import (
	"fmt"
	"image/color"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/OpticalFlyer/goliath/tilemap"
	"github.com/OpticalFlyer/goliath/ui"
)

// Constants for initial view
const (
	initialLat  = 39.8333 // Approx. center of contiguous US (Kansas)
	initialLon  = -98.5833
	initialZoom = 4 // Start with a bit more zoom to see the US
)

// Goliath implements ebiten.Game interface.
type Goliath struct {
	tileMap   *tilemap.TileMap
	debugMode bool
	ui        *ui.Controller

	// Mouse panning state
	isDragging bool
	lastMouseX int
	lastMouseY int

	lastZoomTime float64 // Track last zoom time

	// Touch state for multi-touch interactions
	lastTouchX map[ebiten.TouchID]float64
	lastTouchY map[ebiten.TouchID]float64
}

func (g *Goliath) Update() error {
	// Update UI first to handle any panel interactions
	if err := g.ui.Update(); err != nil {
		return err
	}

	// Only handle map interactions if we're not interacting with UI
	if !g.ui.IsInteractingWithUI() {
		if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
			g.debugMode = !g.debugMode
		}

		// Handle keyboard zooming
		if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || // = key
			inpututil.IsKeyJustPressed(ebiten.KeyNumpadAdd) { // numpad +
			g.tileMap.ZoomIn()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || // - key
			inpututil.IsKeyJustPressed(ebiten.KeyNumpadSubtract) { // numpad -
			g.tileMap.ZoomOut()
		}

		// Handle mouse wheel zooming with time-based throttling
		currentTime := float64(time.Now().UnixNano()) / 1e9 // Current time in seconds
		_, wheelY := ebiten.Wheel()
		if wheelY != 0 && (currentTime-g.lastZoomTime) > 0.1 { // 100ms between zooms
			x, y := ebiten.CursorPosition()
			g.tileMap.ZoomAtPoint(wheelY > 0, float64(x), float64(y))
			g.lastZoomTime = currentTime
		}

		// Handle keyboard panning
		if ebiten.IsKeyPressed(ebiten.KeyLeft) {
			g.tileMap.Pan(tilemap.PanLeft)
		}
		if ebiten.IsKeyPressed(ebiten.KeyRight) {
			g.tileMap.Pan(tilemap.PanRight)
		}
		if ebiten.IsKeyPressed(ebiten.KeyUp) {
			g.tileMap.Pan(tilemap.PanUp)
		}
		if ebiten.IsKeyPressed(ebiten.KeyDown) {
			g.tileMap.Pan(tilemap.PanDown)
		}

		// Handle mouse panning
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			// Start dragging
			g.isDragging = true
			g.lastMouseX, g.lastMouseY = ebiten.CursorPosition()
		} else if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			// Stop dragging
			g.isDragging = false
		}

		if g.isDragging {
			// Get current mouse position
			currentX, currentY := ebiten.CursorPosition()

			// Calculate the difference from last position
			dx := float64(currentX - g.lastMouseX)
			dy := float64(currentY - g.lastMouseY)

			// Pan the map
			if dx != 0 || dy != 0 {
				g.tileMap.PanBy(dx, dy)
			}

			// Update last position
			g.lastMouseX = currentX
			g.lastMouseY = currentY
		}

		// Handle touch events
		g.handleTouchEvents()
	}

	return nil
}

func (g *Goliath) Draw(screen *ebiten.Image) {
	// Draw the tile map and get the visible range for debug info
	tileRange := g.tileMap.Draw(screen, g.debugMode)

	// Draw UI
	g.ui.Draw(screen)

	// Draw debug overlay if enabled
	if g.debugMode {
		redColor := color.RGBA{R: 255, A: 255}
		strokeWidth := float32(1.0)

		// Draw crosshair
		centerX := float32(g.tileMap.ScreenWidth / 2)
		centerY := float32(g.tileMap.ScreenHeight / 2)
		crosshairSize := float32(10.0)

		vector.StrokeLine(screen,
			centerX-crosshairSize, centerY,
			centerX+crosshairSize, centerY,
			strokeWidth, redColor, false)
		vector.StrokeLine(screen,
			centerX, centerY-crosshairSize,
			centerX, centerY+crosshairSize,
			strokeWidth, redColor, false)

		// Draw debug text
		debugText := fmt.Sprintf("Lat: %.4f\nLon: %.4f\nZoom: %d\nTiles: %d,%d - %d,%d",
			g.tileMap.CenterLat, g.tileMap.CenterLon, g.tileMap.Zoom,
			tileRange.MinX, tileRange.MinY, tileRange.MaxX, tileRange.MaxY)
		ebitenutil.DebugPrint(screen, debugText)
	}
}

func (g *Goliath) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.tileMap.ScreenWidth = outsideWidth
	g.tileMap.ScreenHeight = outsideHeight
	g.ui.UpdateWindowSize(outsideWidth, outsideHeight)
	return outsideWidth, outsideHeight
}

func main() {
	uiController := ui.NewController()

	// Create main map control panel
	mapPanel := ui.NewPanel(10, 10, 200, 300, "Map Controls")
	uiController.AddPanel(mapPanel)

	app := &Goliath{
		tileMap:      tilemap.New(800, 600, initialLat, initialLon, initialZoom),
		debugMode:    false,
		ui:           uiController,
		lastZoomTime: float64(time.Now().UnixNano()) / 1e9,
	}

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Goliath")
	//ebiten.SetTPS(ebiten.SyncWithFPS)
	ebiten.SetVsyncEnabled(true)

	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}
