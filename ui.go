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
func (g *Game) handleTextInput() {
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
}

// executeCommand processes and executes user-entered commands
func (g *Game) executeCommand() {
	command := strings.ToUpper(strings.TrimSpace(g.TextBoxText))

	// Check if we're exiting insert mode
	wasInInsertMode := g.insertMode
	if g.insertMode && (command == "" || command != "PO") {
		g.insertMode = false
		fmt.Println("Point insertion mode deactivated")
	}

	// Only handle empty command repeating if we weren't exiting insert mode
	if !wasInInsertMode && command == "" && g.lastCommand != "" {
		command = g.lastCommand
	}

	if command == "" {
		return
	}

	success := true
	switch command {
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
func (g *Game) drawText(screen *ebiten.Image, x, y float64, clr color.RGBA, textStr string) {
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
func (g *Game) DrawTextbox(screen *ebiten.Image, screenWidth, screenHeight int) {
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
	g.drawText(screen, textX, textY, textColor, g.TextBoxText)
}
