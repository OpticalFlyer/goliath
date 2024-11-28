// ui.go
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

//go:embed fonts/Roboto/Roboto-Regular.ttf
var robotoTTF []byte

var robotoFaceSource *text.GoTextFaceSource

func init() {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(robotoTTF))
	if err != nil {
		log.Fatal(err)
	}
	robotoFaceSource = s
}

// handleTextInput processes user text input for commands
func (g *Goliath) handleTextInput() {
	// Get input characters
	chars := ebiten.AppendInputChars(nil)
	for _, char := range chars {
		g.TextBoxText += string(char)
	}

	// Handle backspace
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if len(g.TextBoxText) > 0 {
			g.TextBoxText = g.TextBoxText[:len(g.TextBoxText)-1]
		}
	}

	// Special handling for layer name input
	if g.inLayerCommand && g.layerSubcommand == "N" && g.layerSubprompt == "Enter layer name <enter>: " {
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			// Create new layer with entered name
			newLayer := NewLayer(g.TextBoxText, g.ScreenWidth, g.ScreenHeight)

			if g.currentLayer != nil {
				// Add as child of current layer
				g.currentLayer.AddChild(newLayer)
				g.currentLayer.Expanded = true
			} else {
				// Add as top-level layer
				g.layers = append(g.layers, newLayer)
			}

			g.layerPanel.UpdateLayers(g.layers)

			// Reset layer command state
			g.inLayerCommand = false
			g.layerSubprompt = ""
			g.layerSubcommand = ""
			g.subcommandJustExecuted = true
			g.TextBoxText = ""
			fmt.Printf("Created new layer: %s\n", newLayer.Name)
			return
		}
		// Allow continued input for layer name
		return
	}
}

// executeCommand processes and executes user-entered commands
func (g *Goliath) executeCommand() {
	command := strings.ToUpper(strings.TrimSpace(g.TextBoxText))

	// Check if we're in pan mode and want to exit
	if g.isPanTool && (command == "") {
		g.isPanTool = false
		fmt.Println("Pan mode deactivated")
		return
	}

	// Check if we're exiting insert mode
	wasInInsertMode := g.insertMode
	if g.insertMode && (command == "" || command != "PO") {
		g.insertMode = false
		fmt.Println("Point insertion mode deactivated")
	}

	// Only handle empty command repeating if we weren't exiting any mode
	if command == "" && g.lastCommand != "" &&
		!g.drawingLine && !g.drawingPolygon && !g.isPanTool && !wasInInsertMode {
		command = g.lastCommand
	}

	if command == "" {
		return
	}

	// Regular command processing
	if command == "-LAYER" {
		g.inLayerCommand = true
		g.layerSubprompt = "?/Make/Set/New/ON/OFF/Color/Ltype/LWeight/Plot/Description/Rename/Delete/Copy: "
		g.TextBoxText = ""
		g.lastCommand = command
		return
	}

	// Handle layer subcommand mode
	if g.inLayerCommand {
		if g.layerSubprompt == "?/Make/Set/New/ON/OFF/Color/Ltype/LWeight/Plot/Description/Rename/Delete/Copy: " {
			if command == "N" || command == "NEW" {
				g.layerSubcommand = "N"
				g.layerSubprompt = "Enter layer name <enter>: "
				g.TextBoxText = ""
			} else {
				// Invalid subcommand - exit layer command mode
				g.inLayerCommand = false
				g.layerSubprompt = ""
				g.layerSubcommand = ""
				g.TextBoxText = ""
			}
		}
		return
	}

	success := true
	switch command {
	case "PAN":
		if !g.isPanTool {
			g.isPanTool = true
			fmt.Println("Pan mode activated. Press Enter/Space to exit.")
		} else {
			g.isPanTool = false
			fmt.Println("Pan mode deactivated")
		}
	case "LAYER":
		g.layerPanel.visible = true
		fmt.Println("Layer panel toggled")
	case "DI":
		if !g.measuringDistance {
			g.measuringDistance = true
			g.distancePoints = make([]Point, 0)
			fmt.Println("Distance measuring mode activated. Click to add points. Press Enter/Space to finish.")
		}
	case "DEL":
		// Delete selected geometries from all visible layers using WalkLayers
		for _, rootLayer := range g.layers {
			WalkLayers(rootLayer, func(layer *Layer) {
				if !layer.IsEffectivelyVisible() {
					return
				}

				// Delete selected points
				points := layer.PointLayer.Index.Search(Bounds{
					MinX: -180, MinY: -90,
					MaxX: 180, MaxY: 90,
				})
				newPointIndex := NewRTree()
				for _, p := range points {
					point := p.(*Point)
					if !point.Selected {
						newPointIndex.Insert(point, point.Bounds())
					} else {
						g.clearAffectedTiles(layer, point)
					}
				}
				layer.PointLayer.Index = newPointIndex

				// Delete selected lines
				lines := layer.PolylineLayer.Index.Search(Bounds{
					MinX: -180, MinY: -90,
					MaxX: 180, MaxY: 90,
				})
				newLineIndex := NewRTree()
				for _, l := range lines {
					line := l.(*LineString)
					if !line.Selected {
						newLineIndex.Insert(line, line.Bounds())
					} else {
						g.clearAffectedLineTiles(layer, line)
					}
				}
				layer.PolylineLayer.Index = newLineIndex

				// Delete selected polygons
				polygons := layer.PolygonLayer.Index.Search(Bounds{
					MinX: -180, MinY: -90,
					MaxX: 180, MaxY: 90,
				})
				newPolyIndex := NewRTree()
				for _, p := range polygons {
					polygon := p.(*Polygon)
					if !polygon.Selected { // Fixed: Changed point.Selected to polygon.Selected
						newPolyIndex.Insert(polygon, polygon.Bounds())
					} else {
						g.clearAffectedPolygonTiles(layer, polygon)
					}
				}
				layer.PolygonLayer.Index = newPolyIndex

				// Invalidate layer bounds after deletion
				layer.invalidateBounds()
			})
		}
		g.needRedraw = true
		fmt.Println("Deleted selected geometries")
	case "POL":
		if !g.drawingPolygon {
			g.drawingPolygon = true
			g.polygonPoints = make([]Point, 0)
			fmt.Println("Polygon drawing mode activated. Click to add points. Press Enter/Space to finish.")
		}
	case "RANDPOL":
		go func() {
			fmt.Println("Generating 100,000 random polygons...")
			g.InitializeTestPolygons(g.layers[0], 100000)
			fmt.Println("Polygon generation complete")
		}()
	case "PL":
		if !g.drawingLine {
			g.drawingLine = true
			g.linePoints = make([]Point, 0)
			fmt.Println("Line drawing mode activated. Click to add points. Press Enter/Space to finish.")
		}
	case "RANDPL":
		go func() {
			fmt.Println("Generating 100,000 random lines...")
			g.InitializeTestLines(g.layers[0], 100000)
			fmt.Println("Line generation complete")
		}()
	case "RANDPO":
		go func() {
			fmt.Println("Generating 100,000 random points...")
			g.InitializeTestPoints(g.layers[0], 100000)
			fmt.Println("Point generation complete")
		}()
	case "PO":
		g.insertMode = true
		fmt.Println("Point insertion mode activated. Click to add points. Press Enter/Space to exit.")
	case "GOOGLEHYBRID":
		g.basemap = GOOGLEHYBRID
		ClearDownloadQueue(g.tileCache)
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "GOOGLEAERIAL":
		g.basemap = GOOGLEAERIAL
		ClearDownloadQueue(g.tileCache)
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "BINGHYBRID":
		g.basemap = BINGHYBRID
		ClearDownloadQueue(g.tileCache)
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "BINGAERIAL":
		g.basemap = BINGAERIAL
		ClearDownloadQueue(g.tileCache)
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "OSM":
		g.basemap = OSM
		ClearDownloadQueue(g.tileCache)
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "PLOT":
		err := g.plotToPDF()
		if err != nil {
			fmt.Printf("Error creating PDF: %v\n", err)
			success = false
		} else {
			fmt.Println("PDF created successfully: plot.pdf")
		}
	default:
		success = false
		fmt.Printf("Unknown command: %s\n", command)
	}

	if success {
		g.lastCommand = command
	}

	// Check if the current zoom level exceeds the maximum zoom level for the new basemap
	if g.zoom > maxZoomLevels[g.basemap] {
		g.zoom = maxZoomLevels[g.basemap]
	}
}

// drawText renders text on the screen at specified coordinates with a given color
func (g *Goliath) drawText(screen *ebiten.Image, x, y float64, clr color.RGBA, textStr string) {
	fontFace := &text.GoTextFace{
		Source: robotoFaceSource,
		Size:   16,
	}

	_, h := text.Measure(textStr, fontFace, 0)
	adjustedY := y - h/2

	// Create a DrawOptions struct
	opts := &text.DrawOptions{}
	opts.ColorScale.ScaleWithColor(clr)
	opts.GeoM.Translate(x, adjustedY)

	// Draw the text using the new text/v2 package
	text.Draw(screen, textStr, fontFace, opts)
}

// DrawTextbox renders the command input textbox on the screen
func (g *Goliath) DrawTextbox(screen *ebiten.Image, screenWidth, screenHeight int) {
	// Textbox dimensions
	boxWidth := int(0.8 * float64(screenWidth))
	if boxWidth > 800 {
		boxWidth = 800
	}
	boxHeight := 24
	boxX := (screenWidth - boxWidth) / 2
	boxY := screenHeight - boxHeight - 20

	// Draw textbox background
	bgColor := color.RGBA{50, 50, 50, 200}
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), float32(boxWidth), float32(boxHeight), bgColor, false)

	// Draw textbox border
	borderColor := color.RGBA{255, 255, 255, 255}
	// Top border
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), float32(boxWidth), 1, borderColor, false)
	// Bottom border
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY+boxHeight-1), float32(boxWidth), 1, borderColor, false)
	// Left border
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), 1, float32(boxHeight), borderColor, false)
	// Right border
	vector.DrawFilledRect(screen, float32(boxX+boxWidth-1), float32(boxY), 1, float32(boxHeight), borderColor, false)

	// Draw the input text
	textX := float64(boxX) + 10
	textY := float64(boxY) + float64(boxHeight)/2
	textColor := color.RGBA{255, 255, 255, 255}

	if g.inLayerCommand {
		promptText := g.layerSubprompt + g.TextBoxText
		g.drawText(screen, textX, textY, textColor, promptText)
	} else {
		g.drawText(screen, textX, textY, textColor, g.TextBoxText)
	}
}
