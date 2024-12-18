// main.go
package main

import (
	"container/list"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Goliath struct encapsulates the program state and behavior
type Goliath struct {
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
	isPanTool       bool // Track if pan tool is active

	lastCommand            string // Store the last successful command
	subcommandJustExecuted bool   // Flag to prevent double-execution of commands

	// Fields for point drawing
	insertMode bool // Track if we're in point insertion mode

	// Fields for line drawing
	drawingLine bool
	linePoints  []Point
	lastMouseX  int
	lastMouseY  int

	// Fields for polygon drawing
	drawingPolygon bool
	polygonPoints  []Point

	droppedFiles chan string // Channel for dropped files

	// Selection box
	selectionBoxStart struct {
		x, y     int
		lat, lon float64
	}
	isSelectionDrag bool

	vertexEditState *VertexEditState

	snappingEnabled bool
	snapThreshold   float64 // Snap distance in pixels
	snapTarget      *Point  // Store current snap target vertex

	measuringDistance bool
	distancePoints    []Point

	layers       []*Layer
	layerPanel   *LayerPanel
	currentLayer *Layer

	// Layer command state
	inLayerCommand  bool
	layerSubprompt  string
	layerSubcommand string

	// Add KML style-related fields
	StyleMap   map[string]map[string]string
	Styles     map[string]PolyLineStyle
	IconStyles map[string]IconStyleData
	IconImages map[string]*ebiten.Image

	defaultRender bool
	showGrid      bool
	showBounds    bool

	lastZoom       int
	lastZoomChange int64 // Frame counter when zoom last changed
	frameCount     int64
}

func (g *Goliath) isZoomStable() bool {
	return g.frameCount-g.lastZoomChange > 10 // Adjust frames threshold as needed
}

// Initialize sets up the initial state of the program
func Initialize() (*Goliath, error) {
	// Initialize the cache with a maximum of 10000 tiles
	tileCache := NewTileImageCache(10000)

	g := &Goliath{
		centerLat:       39.8283, // Center of the US
		centerLon:       -98.5795,
		zoom:            5,            // Default zoom level
		basemap:         GOOGLEAERIAL, // Default basemap
		ScreenWidth:     1024,
		ScreenHeight:    768,
		tileCache:       tileCache, // tileCache is *TileImageCache
		needRedraw:      true,
		snappingEnabled: true,
		snapThreshold:   10.0, // 10 pixel snap radius
		layers:          make([]*Layer, 0),
		defaultRender:   false,
		showGrid:        false,
		showBounds:      false,
	}

	// Initialize an empty tile (solid color) for missing tiles
	g.emptyTile = ebiten.NewImage(256, 256)
	solidColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	g.emptyTile.Fill(solidColor)

	g.StyleMap = make(map[string]map[string]string)
	g.Styles = make(map[string]PolyLineStyle)
	g.IconStyles = make(map[string]IconStyleData)
	g.IconImages = make(map[string]*ebiten.Image)

	g.offscreenImage = ebiten.NewImage(g.ScreenWidth, g.ScreenHeight)

	g.droppedFiles = make(chan string, 1)

	// Create root layer
	rootLayer := NewLayer("Root", g.ScreenWidth, g.ScreenHeight)
	g.layers = append(g.layers, rootLayer)
	g.currentLayer = rootLayer // Set root as initial current layer

	g.layerPanel = NewLayerPanel(0, 165, g.layers, g)

	return g, nil
}

// Update handles the program state updates, including panning, zooming, and UI interactions
func (g *Goliath) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyG) && ebiten.IsKeyPressed(ebiten.KeyControl) {
		g.showGrid = !g.showGrid
		fmt.Printf("Show grid: %v\n", g.showGrid)
		g.needRedraw = true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyB) && ebiten.IsKeyPressed(ebiten.KeyControl) {
		g.showBounds = !g.showBounds
		fmt.Printf("Show bounds: %v\n", g.showBounds)
		g.needRedraw = true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyD) && ebiten.IsKeyPressed(ebiten.KeyControl) {
		g.defaultRender = !g.defaultRender
		fmt.Printf("Default rendering: %v\n", g.defaultRender)

		// Clear all tile caches to force redraw
		for _, layer := range g.layers {
			WalkLayers(layer, func(l *Layer) {
				l.PointTileCache.mu.Lock()
				l.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
				l.PointTileCache.lruList = list.New()
				l.PointTileCache.lruMap = make(map[string]*list.Element)
				l.PointTileCache.mu.Unlock()

				l.LineTileCache.mu.Lock()
				l.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
				l.LineTileCache.lruList = list.New()
				l.LineTileCache.lruMap = make(map[string]*list.Element)
				l.LineTileCache.mu.Unlock()
			})
		}
		g.needRedraw = true
	}

	// Change cursor appearance when pan tool is active
	if g.isPanTool {
		ebiten.SetCursorShape(ebiten.CursorShapeMove)
	} else {
		ebiten.SetCursorShape(ebiten.CursorShapeDefault)
	}

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
	if !g.subcommandJustExecuted && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		(inpututil.IsKeyJustPressed(ebiten.KeySpace) &&
			!(g.inLayerCommand && g.layerSubcommand == "N" && g.layerSubprompt == "Enter layer name <enter>: "))) {
		g.executeCommand()
		g.TextBoxText = "" // Clear the textbox after executing
	}
	g.subcommandJustExecuted = false

	// Toggle snapping with F3
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.snappingEnabled = !g.snappingEnabled
		fmt.Printf("Snapping %s\n", map[bool]string{true: "enabled", false: "disabled"}[g.snappingEnabled])
	}

	// Handle panning with either middle mouse OR (shift+left click OR pan tool+left click)
	isPanningAction := ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) ||
		(ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && g.isPanTool)

	if isPanningAction {
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

	if !isPanningAction {

		// Update snap target during mouse movement
		if g.snappingEnabled {
			mouseX, mouseY := ebiten.CursorPosition()
			if target, found := g.findNearestVertex(mouseX, mouseY); found {
				g.snapTarget = target
			} else {
				g.snapTarget = nil
			}
		} else {
			g.snapTarget = nil
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
		g.frameCount++
		if _, scrollY := ebiten.Wheel(); scrollY != 0 {
			// Record when zoom changed
			if g.zoom != g.lastZoom {
				g.lastZoomChange = g.frameCount
				g.lastZoom = g.zoom
			}

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

		// Handle point insertion mode
		if g.insertMode {
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				mouseX, mouseY := ebiten.CursorPosition()
				lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

				if g.snappingEnabled {
					if nearest, found := g.findNearestVertex(mouseX, mouseY); found {
						lat, lon = nearest.Lat, nearest.Lon
					}
				}

				point := NewPoint(lat, lon)
				g.currentLayer.PointLayer.Index.Insert(point, point.Bounds())
				g.clearAffectedTiles(g.currentLayer, point)
			}
		}

		if g.drawingLine {
			mouseX, mouseY := ebiten.CursorPosition()
			g.lastMouseX, g.lastMouseY = mouseX, mouseY // Update for preview

			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

				// Apply snapping if enabled
				if g.snappingEnabled {
					if nearest, found := g.findNearestVertex(mouseX, mouseY); found {
						lat, lon = nearest.Lat, nearest.Lon
					}
				}

				g.linePoints = append(g.linePoints, Point{Lat: lat, Lon: lon})
				g.needRedraw = true
			}

			// Only check for completion if we have at least one point
			if len(g.linePoints) > 0 &&
				(inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace)) {
				if len(g.linePoints) >= 2 {
					line := &LineString{Points: g.linePoints}
					g.currentLayer.PolylineLayer.Index.Insert(line, line.Bounds())
					g.clearAffectedLineTiles(g.currentLayer, line)
				}
				g.drawingLine = false
				g.linePoints = nil
				g.needRedraw = true
				fmt.Println("Line drawing completed")
			}
		}

		if g.drawingPolygon {
			mouseX, mouseY := ebiten.CursorPosition()

			// Handle mouse clicks to add points
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

				// Apply snapping if enabled
				if g.snappingEnabled {
					if nearest, found := g.findNearestVertex(mouseX, mouseY); found {
						lat, lon = nearest.Lat, nearest.Lon
					}
				}

				g.polygonPoints = append(g.polygonPoints, Point{Lat: lat, Lon: lon})
				g.needRedraw = true
			}

			// Check for completion (similar to PL logic)
			if len(g.polygonPoints) > 0 &&
				(inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace)) {
				if len(g.polygonPoints) >= 3 {
					polygon := &Polygon{Points: g.polygonPoints}
					g.currentLayer.PolygonLayer.Index.Insert(polygon, polygon.Bounds())
					g.clearAffectedPolygonTiles(g.currentLayer, polygon)
				}
				g.drawingPolygon = false
				g.polygonPoints = nil
				g.needRedraw = true
				fmt.Println("Polygon drawing completed")
			}
		}

		// Handle dropped files
		if files := ebiten.DroppedFiles(); files != nil {
			fmt.Printf("Dropped files: %v\n", files)

			go func() {
				err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}

					ext := strings.ToLower(filepath.Ext(path))

					// Handle shapefiles
					if ext == ".shp" {
						log.Printf("Processing shapefile: %s", path)

						// Copy .shp, .shx, .dbf files to temp directory
						baseName := strings.TrimSuffix(path, ".shp")
						exts := []string{".shp", ".shx", ".dbf"}
						tempDir, err := os.MkdirTemp("", "shapefile")
						if err != nil {
							log.Printf("Error creating temp directory: %v", err)
							return nil
						}
						// Clean up temp directory later
						//defer os.RemoveAll(tempDir)

						for _, ext := range exts {
							virtualFilePath := baseName + ext
							f, err := files.Open(virtualFilePath)
							if err != nil {
								log.Printf("Error opening %s: %v", virtualFilePath, err)
								continue
							}
							defer f.Close()

							tempFilePath := filepath.Join(tempDir, filepath.Base(virtualFilePath))
							tempFile, err := os.Create(tempFilePath)
							if err != nil {
								log.Printf("Error creating temp file %s: %v", tempFilePath, err)
								continue
							}

							if _, err := io.Copy(tempFile, f); err != nil {
								log.Printf("Error copying to temp file %s: %v", tempFilePath, err)
								tempFile.Close()
								continue
							}
							tempFile.Close()
						}

						// Now load the shapefile from the temp directory
						shpPath := filepath.Join(tempDir, filepath.Base(path))
						go g.loadShapefile(shpPath)
					}

					// KML/KMZ handling
					if ext == ".kml" || ext == ".kmz" {
						log.Printf("Processing KML/KMZ file: %s", path)

						// Get file content
						f, err := files.Open(path)
						if err != nil {
							log.Printf("Error opening %s: %v", path, err)
							return nil
						}
						defer f.Close()

						// Create new layer for the KML data
						filename := filepath.Base(path)
						layerName := strings.TrimSuffix(filename, filepath.Ext(filename))
						newLayer := NewLayer(layerName, g.ScreenWidth, g.ScreenHeight)

						// Add layer as child of current layer if one is selected
						if g.currentLayer != nil {
							g.currentLayer.AddChild(newLayer)
							g.currentLayer.Expanded = true
						} else {
							g.layers = append(g.layers, newLayer)
						}

						// Update layer panel
						g.layerPanel.UpdateLayers(g.layers)
						g.layerPanel.visible = true

						// Load the KML into the new layer
						err = LoadKMLDroppedFiles(files, g, newLayer)
						if err != nil {
							log.Printf("Error loading KML file: %v", err)
							return nil
						} else {
							newLayer.PointTileCache.mu.Lock()
							newLayer.PointTileCache.cache = make(map[int]map[int]map[int]*PointTile)
							newLayer.PointTileCache.lruList = list.New()
							newLayer.PointTileCache.lruMap = make(map[string]*list.Element)
							newLayer.PointTileCache.mu.Unlock()

							newLayer.LineTileCache.mu.Lock()
							newLayer.LineTileCache.cache = make(map[int]map[int]map[int]*LineTile)
							newLayer.LineTileCache.lruList = list.New()
							newLayer.LineTileCache.lruMap = make(map[string]*list.Element)
							newLayer.LineTileCache.mu.Unlock()

							newLayer.PolygonTileCache.mu.Lock()
							newLayer.PolygonTileCache.cache = make(map[int]map[int]map[int]*PolygonTile)
							newLayer.PolygonTileCache.lruList = list.New()
							newLayer.PolygonTileCache.lruMap = make(map[string]*list.Element)
							newLayer.PolygonTileCache.mu.Unlock()

							g.needRedraw = true
						}
					}
					return nil
				})
				if err != nil {
					log.Printf("Error processing dropped files: %v", err)
				}
			}()
		}

		// Handle selections
		if !g.drawingLine && !g.drawingPolygon && !g.insertMode && !g.isDragging {
			// Add check for vertex editing
			isVertexEditing := g.vertexEditState != nil &&
				(g.vertexEditState.HoveredVertexID >= 0 ||
					g.vertexEditState.HoveredInsertionID >= 0 ||
					g.vertexEditState.DragState.IsEditing ||
					g.vertexEditState.InsertionDragState.IsEditing)

			// Add check for layer panel interaction
			mouseX, mouseY := ebiten.CursorPosition()
			isOverLayerPanel := g.layerPanel.visible &&
				mouseX >= g.layerPanel.x &&
				mouseX <= g.layerPanel.x+g.layerPanel.width &&
				mouseY >= g.layerPanel.y &&
				mouseY <= g.layerPanel.y+g.layerPanel.height

			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && !isVertexEditing && !isOverLayerPanel {
				mouseX, mouseY := ebiten.CursorPosition()
				lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
				g.selectionBoxStart.x = mouseX
				g.selectionBoxStart.y = mouseY
				g.selectionBoxStart.lat = lat
				g.selectionBoxStart.lon = lon
				g.isSelectionDrag = true
			}

			if g.isSelectionDrag && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
				// Mouse released - determine if this was a click or drag
				mouseX, mouseY := ebiten.CursorPosition()
				dragDistance := math.Sqrt(math.Pow(float64(mouseX-g.selectionBoxStart.x), 2) +
					math.Pow(float64(mouseY-g.selectionBoxStart.y), 2))

				if ebiten.IsKeyPressed(ebiten.KeyShift) {
					fmt.Println("Shift key is pressed")
				}

				if !isVertexEditing {
					// Treat as point-click if moved less than 5 pixels
					if dragDistance < 5 && ebiten.IsKeyPressed(ebiten.KeyShift) { // Point selection
						// Point-click selection with buffer
						const pixelBuffer = 5.0
						minLat, minLon := latLngFromPixel(float64(mouseX-pixelBuffer), float64(mouseY+pixelBuffer), g)
						maxLat, maxLon := latLngFromPixel(float64(mouseX+pixelBuffer), float64(mouseY-pixelBuffer), g)

						searchBounds := Bounds{
							MinX: math.Min(minLon, maxLon),
							MinY: math.Min(minLat, maxLat),
							MaxX: math.Max(minLon, maxLon),
							MaxY: math.Max(minLat, maxLat),
						}

						clickLat, clickLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

						// Search through layers recursively
						for _, rootLayer := range g.layers {
							WalkLayers(rootLayer, func(layer *Layer) {
								if !layer.IsEffectivelyVisible() {
									return
								}

								// Check points
								points := layer.PointLayer.Index.Search(searchBounds)
								for _, p := range points {
									point := p.(*Point)
									if point.containsPoint(clickLat, clickLon, g.zoom) {
										point.Selected = !point.Selected
										g.clearAffectedTiles(layer, point)
									}
								}

								// Check lines
								lines := layer.PolylineLayer.Index.Search(searchBounds)
								for _, l := range lines {
									line := l.(*LineString)
									if line.containsPoint(clickLat, clickLon, g.zoom) {
										line.Selected = !line.Selected
										g.clearAffectedLineTiles(layer, line)
									}
								}

								// Check polygons
								polygons := layer.PolygonLayer.Index.Search(searchBounds)
								for _, p := range polygons {
									polygon := p.(*Polygon)
									if polygon.containsPoint(clickLat, clickLon, g.zoom) {
										polygon.Selected = !polygon.Selected
										g.clearAffectedPolygonTiles(layer, polygon)
									}
								}
							})
						}
					} else if dragDistance >= 5 { // Box selection
						// Calculate box bounds
						endLat, endLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
						bounds := Bounds{
							MinX: math.Min(g.selectionBoxStart.lon, endLon),
							MinY: math.Min(g.selectionBoxStart.lat, endLat),
							MaxX: math.Max(g.selectionBoxStart.lon, endLon),
							MaxY: math.Max(g.selectionBoxStart.lat, endLat),
						}

						// Search through layers recursively
						for _, rootLayer := range g.layers {
							WalkLayers(rootLayer, func(layer *Layer) {
								if !layer.IsEffectivelyVisible() {
									return
								}

								// Select points in box
								points := layer.PointLayer.Index.Search(bounds)
								for _, p := range points {
									point := p.(*Point)
									point.Selected = true
									g.clearAffectedTiles(layer, point)
								}

								// Select lines in box
								lines := layer.PolylineLayer.Index.Search(bounds)
								for _, l := range lines {
									line := l.(*LineString)
									if line.intersectsBox(bounds) {
										line.Selected = true
										g.clearAffectedLineTiles(layer, line)
									}
								}

								// Select polygons in box
								polygons := layer.PolygonLayer.Index.Search(bounds)
								for _, p := range polygons {
									polygon := p.(*Polygon)
									if polygon.intersectsBox(bounds) {
										polygon.Selected = true
										g.clearAffectedPolygonTiles(layer, polygon)
									}
								}
							})
						}
					}
				}

				g.isSelectionDrag = false
				g.needRedraw = true
			}

			err := g.layerPanel.Update()
			if err != nil {
				return err
			}
		}

		// Clear selections and modes when Escape is released
		if inpututil.IsKeyJustReleased(ebiten.KeyEscape) {
			if g.insertMode || g.drawingLine || g.drawingPolygon || g.isPanTool {
				// Cancel active drawing commands
				if g.insertMode {
					g.insertMode = false
					fmt.Println("Point insertion mode canceled")
				}
				if g.drawingLine {
					g.drawingLine = false
					g.linePoints = nil
					fmt.Println("Line drawing canceled")
					g.needRedraw = true
				}
				if g.drawingPolygon {
					g.drawingPolygon = false
					g.polygonPoints = nil
					fmt.Println("Polygon drawing canceled")
					g.needRedraw = true
				}
				// Cancel pan tool
				if g.isPanTool {
					g.isPanTool = false
					fmt.Println("Pan mode canceled")
					ebiten.SetCursorShape(ebiten.CursorShapeDefault)
				}
			} else if g.vertexEditState != nil && (g.vertexEditState.DragState.IsEditing || g.vertexEditState.InsertionDragState.IsEditing) {
				// Cancel vertex editing
				if g.vertexEditState.DragState.IsEditing {
					// Find layer containing the edited geometry
					var targetLayer *Layer
					for _, layer := range g.layers {
						if !layer.Visible {
							continue
						}

						// Reset vertex position to original
						if g.vertexEditState.EditingPoint != nil {
							points := layer.PointLayer.Index.Search(g.vertexEditState.EditingPoint.Bounds())
							for _, p := range points {
								if p == g.vertexEditState.EditingPoint {
									targetLayer = layer
									g.vertexEditState.EditingPoint.Lat = g.vertexEditState.DragState.OriginalPoint.Lat
									g.vertexEditState.EditingPoint.Lon = g.vertexEditState.DragState.OriginalPoint.Lon
									g.clearAffectedTiles(layer, g.vertexEditState.EditingPoint)
									break
								}
							}
						} else if g.vertexEditState.EditingLine != nil {
							lines := layer.PolylineLayer.Index.Search(g.vertexEditState.EditingLine.Bounds())
							for _, l := range lines {
								if l == g.vertexEditState.EditingLine {
									targetLayer = layer
									g.vertexEditState.EditingLine.Points[g.vertexEditState.HoveredVertexID] = g.vertexEditState.DragState.OriginalPoint
									g.clearAffectedLineTiles(layer, g.vertexEditState.EditingLine)
									break
								}
							}
						} else if g.vertexEditState.EditingPolygon != nil {
							polygons := layer.PolygonLayer.Index.Search(g.vertexEditState.EditingPolygon.Bounds())
							for _, p := range polygons {
								if p == g.vertexEditState.EditingPolygon {
									targetLayer = layer
									g.vertexEditState.EditingPolygon.Points[g.vertexEditState.HoveredVertexID] = g.vertexEditState.DragState.OriginalPoint
									g.clearAffectedPolygonTiles(layer, g.vertexEditState.EditingPolygon)
									break
								}
							}
						}

						if targetLayer != nil {
							break
						}
					}
				} else if g.vertexEditState.InsertionDragState.IsEditing {
					// Cancel insertion drag
					g.vertexEditState.InsertionDragState.IsEditing = false
					fmt.Println("Vertex insertion canceled")
				}

				// Reset vertex edit state
				g.vertexEditState = nil
				g.needRedraw = true
			} else {
				// Clear all selections using WalkLayers
				for _, rootLayer := range g.layers {
					WalkLayers(rootLayer, func(layer *Layer) {
						if !layer.IsEffectivelyVisible() {
							return
						}

						// Clear point selections
						points := layer.PointLayer.Index.Search(Bounds{
							MinX: -180,
							MinY: -90,
							MaxX: 180,
							MaxY: 90,
						})
						for _, p := range points {
							point := p.(*Point)
							if point.Selected {
								point.Selected = false
								g.clearAffectedTiles(layer, point)
							}
						}

						// Clear line selections
						lines := layer.PolylineLayer.Index.Search(Bounds{
							MinX: -180,
							MinY: -90,
							MaxX: 180,
							MaxY: 90,
						})
						for _, l := range lines {
							line := l.(*LineString)
							if line.Selected {
								line.Selected = false
								g.clearAffectedLineTiles(layer, line)
							}
						}

						// Clear polygon selections
						polygons := layer.PolygonLayer.Index.Search(Bounds{
							MinX: -180,
							MinY: -90,
							MaxX: 180,
							MaxY: 90,
						})
						for _, p := range polygons {
							polygon := p.(*Polygon)
							if polygon.Selected {
								polygon.Selected = false
								g.clearAffectedPolygonTiles(layer, polygon)
							}
						}
					})
				}

				g.needRedraw = true
			}
		}

		// Handle vertex editing mode - but only if not dragging a selection box
		if !g.isDragging && !g.drawingLine && !g.drawingPolygon && !g.insertMode && !g.isSelectionDrag {
			mouseX, mouseY := ebiten.CursorPosition()

			// Layer panel check
			isOverLayerPanel := g.layerPanel.visible &&
				mouseX >= g.layerPanel.x &&
				mouseX <= g.layerPanel.x+g.layerPanel.width &&
				mouseY >= g.layerPanel.y &&
				mouseY <= g.layerPanel.y+g.layerPanel.height

			// Only find hovered geometry if not over layer panel
			if !isOverLayerPanel {
				g.findHoveredGeometry(mouseX, mouseY)
			} else {
				// Clear vertex edit state when over panel
				g.vertexEditState = nil
			}
		}

		// Handle vertex editing mouse interactions - but only if not dragging a selection box
		if !g.isDragging && !g.drawingLine && !g.drawingPolygon && !g.insertMode && !g.isSelectionDrag {
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				mouseX, mouseY := ebiten.CursorPosition()
				if g.vertexEditState != nil && !ebiten.IsKeyPressed(ebiten.KeyShift) {
					if g.vertexEditState.DragState.IsEditing {
						g.finishVertexEdit(mouseX, mouseY)
					} else if g.vertexEditState.InsertionDragState.IsEditing {
						g.finishInsertionDrag(mouseX, mouseY)
					} else if g.vertexEditState.HoveredVertexID >= 0 {
						g.startVertexDrag()
					} else if g.vertexEditState.HoveredInsertionID >= 0 {
						g.startInsertionDrag()
					}
				}
			}
		}

		// Handle vertex deletion with right click
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			if g.vertexEditState != nil && g.vertexEditState.HoveredVertexID >= 0 {
				g.deleteVertex()
				g.needRedraw = true
			}
		}

		// Handle distance measuring
		if g.measuringDistance {
			mouseX, mouseY := ebiten.CursorPosition()

			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				lat, lon := latLngFromPixel(float64(mouseX), float64(mouseY), g)

				// Apply snapping if enabled
				if g.snappingEnabled {
					if nearest, found := g.findNearestVertex(mouseX, mouseY); found {
						lat, lon = nearest.Lat, nearest.Lon
					}
				}

				g.distancePoints = append(g.distancePoints, Point{Lat: lat, Lon: lon})

				// Calculate and display segment distance if we have at least 2 points
				if len(g.distancePoints) >= 2 {
					last := g.distancePoints[len(g.distancePoints)-1]
					prev := g.distancePoints[len(g.distancePoints)-2]
					segmentDist := haversineDistance(prev.Lat, prev.Lon, last.Lat, last.Lon)
					fmt.Printf("Segment distance: %d feet\n", roundToNearestFoot(segmentDist))
				}

				g.needRedraw = true
			}

			// Check for completion
			if len(g.distancePoints) > 0 &&
				(inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace)) {
				// Calculate total distance
				totalDist := 0.0
				for i := 1; i < len(g.distancePoints); i++ {
					p1 := g.distancePoints[i-1]
					p2 := g.distancePoints[i]
					totalDist += haversineDistance(p1.Lat, p1.Lon, p2.Lat, p2.Lon)
				}
				fmt.Printf("Total distance: %d feet\n", roundToNearestFoot(totalDist))

				g.measuringDistance = false
				g.distancePoints = nil
				g.needRedraw = true
				fmt.Println("Distance measuring completed")
			}
		}
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

	// Draw tile grid if enabled
	if g.showGrid {
		g.drawTileGrid(screen)
		g.drawCenterCrosshair(screen)
	}

	// Draw geometry layers
	g.DrawPolygons(screen)
	g.DrawLines(screen)
	g.DrawPoints(screen)

	// Draw bounds if enabled
	if g.showBounds {
		for _, rootLayer := range g.layers {
			WalkLayers(rootLayer, func(layer *Layer) {
				if layer.IsEffectivelyVisible() {
					g.drawLayerBounds(screen, layer)
				}
			})
		}
	}

	// Draw vertex handles if in editing mode
	g.drawVertexHandles(screen)

	// Draw vertex drag preview
	g.drawDragPreview(screen)

	// Draw temporary line if in drawing mode
	if g.drawingLine {
		mouseX, mouseY := ebiten.CursorPosition()

		// Draw existing line segments
		for i := 0; i < len(g.linePoints)-1; i++ {
			p1 := g.linePoints[i]
			p2 := g.linePoints[i+1]

			x1, y1 := latLngToPixel(p1.Lat, p1.Lon, g.zoom)
			x2, y2 := latLngToPixel(p2.Lat, p2.Lon, g.zoom)

			// Convert to screen coordinates
			centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
			screenX1 := x1 - (centerX - float64(g.ScreenWidth)/2)
			screenY1 := y1 - (centerY - float64(g.ScreenHeight)/2)
			screenX2 := x2 - (centerX - float64(g.ScreenWidth)/2)
			screenY2 := y2 - (centerY - float64(g.ScreenHeight)/2)

			vector.StrokeLine(screen,
				float32(screenX1), float32(screenY1),
				float32(screenX2), float32(screenY2),
				2, color.RGBA{0, 0, 255, 255}, false)
		}

		// Draw temporary line from last point to cursor
		if len(g.linePoints) > 0 {
			lastPoint := g.linePoints[len(g.linePoints)-1]
			lastX, lastY := latLngToPixel(lastPoint.Lat, lastPoint.Lon, g.zoom)

			// Convert to screen coordinates
			centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)
			screenLastX := lastX - (centerX - float64(g.ScreenWidth)/2)
			screenLastY := lastY - (centerY - float64(g.ScreenHeight)/2)

			vector.StrokeLine(screen,
				float32(screenLastX), float32(screenLastY),
				float32(mouseX), float32(mouseY),
				2, color.RGBA{0, 0, 255, 128}, false)
		}
	}

	// Draw temporary polygon if in drawing mode
	if g.drawingPolygon {
		mouseX, mouseY := ebiten.CursorPosition()

		// Convert points to screen coordinates for drawing
		centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

		// Draw filled preview polygon if we have at least 2 points
		if len(g.polygonPoints) >= 2 {
			// Create temporary points array including mouse position
			tempPoints := make([]Point, len(g.polygonPoints)+1)
			copy(tempPoints, g.polygonPoints)
			mouseLat, mouseLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
			tempPoints[len(tempPoints)-1] = Point{Lat: mouseLat, Lon: mouseLon}

			// Convert all points to screen coordinates
			vertices := make([]ebiten.Vertex, len(tempPoints))
			for i, pt := range tempPoints {
				x, y := latLngToPixel(pt.Lat, pt.Lon, g.zoom)
				screenX := x - (centerX - float64(g.ScreenWidth)/2)
				screenY := y - (centerY - float64(g.ScreenHeight)/2)

				vertices[i] = ebiten.Vertex{
					DstX:   float32(screenX),
					DstY:   float32(screenY),
					SrcX:   1,
					SrcY:   1,
					ColorR: 0,
					ColorG: 1,
					ColorB: 0,
					ColorA: 0.2,
				}
			}

			// Use the same ear clipping triangulation as final polygons
			indices := triangulatePolygon(tempPoints)

			// Draw the filled polygon
			screen.DrawTriangles(vertices, indices, whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), nil)
		}

		// Draw lines between existing points and to cursor
		for i := 0; i < len(g.polygonPoints); i++ {
			p1 := g.polygonPoints[i]
			x1, y1 := latLngToPixel(p1.Lat, p1.Lon, g.zoom)
			screenX1 := x1 - (centerX - float64(g.ScreenWidth)/2)
			screenY1 := y1 - (centerY - float64(g.ScreenHeight)/2)

			// Draw line to next point
			var screenX2, screenY2 float64
			if i == len(g.polygonPoints)-1 {
				// Last point connects to cursor
				screenX2 = float64(mouseX)
				screenY2 = float64(mouseY)
			} else {
				// Connect to next point
				p2 := g.polygonPoints[i+1]
				x2, y2 := latLngToPixel(p2.Lat, p2.Lon, g.zoom)
				screenX2 = x2 - (centerX - float64(g.ScreenWidth)/2)
				screenY2 = y2 - (centerY - float64(g.ScreenHeight)/2)
			}

			vector.StrokeLine(screen,
				float32(screenX1), float32(screenY1),
				float32(screenX2), float32(screenY2),
				2, color.RGBA{0, 255, 0, 255}, false)
		}

		// Draw line from cursor to first point if we have points
		if len(g.polygonPoints) > 0 {
			firstPoint := g.polygonPoints[0]
			x1, y1 := latLngToPixel(firstPoint.Lat, firstPoint.Lon, g.zoom)
			screenX1 := x1 - (centerX - float64(g.ScreenWidth)/2)
			screenY1 := y1 - (centerY - float64(g.ScreenHeight)/2)

			vector.StrokeLine(screen,
				float32(mouseX), float32(mouseY),
				float32(screenX1), float32(screenY1),
				2, color.RGBA{0, 255, 0, 255}, false)
		}
	}

	// Draw selection box if dragging
	if g.isSelectionDrag {
		mouseX, mouseY := ebiten.CursorPosition()

		// Draw semi-transparent fill
		vector.DrawFilledRect(screen,
			float32(math.Min(float64(g.selectionBoxStart.x), float64(mouseX))),
			float32(math.Min(float64(g.selectionBoxStart.y), float64(mouseY))),
			float32(math.Abs(float64(mouseX-g.selectionBoxStart.x))),
			float32(math.Abs(float64(mouseY-g.selectionBoxStart.y))),
			color.RGBA{100, 100, 255, 64},
			false)

		// Draw outline
		vector.StrokeRect(screen,
			float32(math.Min(float64(g.selectionBoxStart.x), float64(mouseX))),
			float32(math.Min(float64(g.selectionBoxStart.y), float64(mouseY))),
			float32(math.Abs(float64(mouseX-g.selectionBoxStart.x))),
			float32(math.Abs(float64(mouseY-g.selectionBoxStart.y))),
			2,
			color.RGBA{100, 100, 255, 255},
			false)
	}

	// Draw snap indicator after geometry but before UI elements
	g.drawSnapIndicator(screen)

	// Draw distance measuring line if active
	if g.measuringDistance {
		mouseX, mouseY := ebiten.CursorPosition()

		// Convert center coordinates for screen space conversion
		centerX, centerY := latLngToPixel(g.centerLat, g.centerLon, g.zoom)

		// Draw existing line segments
		for i := 0; i < len(g.distancePoints)-1; i++ {
			p1 := g.distancePoints[i]
			p2 := g.distancePoints[i+1]

			x1, y1 := latLngToPixel(p1.Lat, p1.Lon, g.zoom)
			x2, y2 := latLngToPixel(p2.Lat, p2.Lon, g.zoom)

			// Convert to screen coordinates
			screenX1 := x1 - (centerX - float64(g.ScreenWidth)/2)
			screenY1 := y1 - (centerY - float64(g.ScreenHeight)/2)
			screenX2 := x2 - (centerX - float64(g.ScreenWidth)/2)
			screenY2 := y2 - (centerY - float64(g.ScreenHeight)/2)

			// Draw line segment
			vector.StrokeLine(screen,
				float32(screenX1), float32(screenY1),
				float32(screenX2), float32(screenY2),
				2, color.RGBA{255, 255, 0, 255}, false)

			// Draw distance label
			dist := haversineDistance(p1.Lat, p1.Lon, p2.Lat, p2.Lon)
			midX := (screenX1 + screenX2) / 2
			midY := (screenY1 + screenY2) / 2
			g.drawText(screen, midX, midY-10, color.RGBA{255, 255, 0, 255},
				fmt.Sprintf("%d ft", roundToNearestFoot(dist)))
		}

		// Draw line from last point to cursor
		if len(g.distancePoints) > 0 {
			lastPoint := g.distancePoints[len(g.distancePoints)-1]
			lastX, lastY := latLngToPixel(lastPoint.Lat, lastPoint.Lon, g.zoom)

			// Convert to screen coordinates
			screenLastX := lastX - (centerX - float64(g.ScreenWidth)/2)
			screenLastY := lastY - (centerY - float64(g.ScreenHeight)/2)

			// Draw temporary line to cursor
			vector.StrokeLine(screen,
				float32(screenLastX), float32(screenLastY),
				float32(mouseX), float32(mouseY),
				2, color.RGBA{255, 255, 0, 128}, false)

			// Draw temporary distance
			cursorLat, cursorLon := latLngFromPixel(float64(mouseX), float64(mouseY), g)
			tempDist := haversineDistance(lastPoint.Lat, lastPoint.Lon, cursorLat, cursorLon)
			midX := (float64(mouseX) + screenLastX) / 2
			midY := (float64(mouseY) + screenLastY) / 2
			g.drawText(screen, midX, midY-10, color.RGBA{255, 255, 0, 128},
				fmt.Sprintf("%d ft", roundToNearestFoot(tempDist)))
		}
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
	debugBg := ebiten.NewImage(200, 200)
	debugBg.Fill(color.RGBA{0, 0, 0, 128})
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(debugBg, op)

	// Display the debug information
	ebitenutil.DebugPrint(screen, debugString)

	// Draw layer panel last so it appears on top
	g.layerPanel.Draw(screen)
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
