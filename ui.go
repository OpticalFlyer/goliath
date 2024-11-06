// ui.go
package main

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

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
	if command == "" {
		return
	}

	switch command {
	case "GOOGLEHYBRID":
		g.basemap = GOOGLEHYBRID
		ClearDownloadQueue()
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "GOOGLEAERIAL":
		g.basemap = GOOGLEAERIAL
		ClearDownloadQueue()
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "BINGHYBRID":
		g.basemap = BINGHYBRID
		ClearDownloadQueue()
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "BINGAERIAL":
		g.basemap = BINGAERIAL
		ClearDownloadQueue()
		g.tileCache.ClearCache()
		g.needRedraw = true
	case "OSM":
		g.basemap = OSM
		ClearDownloadQueue()
		g.tileCache.ClearCache()
		g.needRedraw = true
	default:
		// Optionally handle unknown commands
		fmt.Printf("Unknown command: %s\n", command)
	}
}

// drawText renders text on the screen at specified coordinates with a given color
func (g *Game) drawText(screen *ebiten.Image, x, y float64, clr color.Color, textStr string) {
	fontFace := basicfont.Face7x13
	textHeight := fontFace.Metrics().Height.Ceil()
	adjustedY := y + float64(fontFace.Metrics().Ascent.Ceil()) - float64(textHeight)/2
	text.Draw(screen, textStr, fontFace, int(x), int(adjustedY), clr)
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
	ebitenutil.DrawRect(screen, float64(boxX), float64(boxY), float64(boxWidth), float64(boxHeight), bgColor)

	// Draw textbox border
	borderColor := color.RGBA{255, 255, 255, 255}
	ebitenutil.DrawRect(screen, float64(boxX), float64(boxY), float64(boxWidth), 1, borderColor)             // Top
	ebitenutil.DrawRect(screen, float64(boxX), float64(boxY+boxHeight-1), float64(boxWidth), 1, borderColor) // Bottom
	ebitenutil.DrawRect(screen, float64(boxX), float64(boxY), 1, float64(boxHeight), borderColor)            // Left
	ebitenutil.DrawRect(screen, float64(boxX+boxWidth-1), float64(boxY), 1, float64(boxHeight), borderColor) // Right

	// Draw the input text
	textX := float64(boxX) + 10
	textY := float64(boxY) + float64(boxHeight)/2
	textColor := color.White
	g.drawText(screen, textX, textY, textColor, g.TextBoxText)
}
