// layerpanel.go
package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	layerRowHeight = 30
	headerHeight   = 30
	checkboxSize   = 16
)

type LayerPanel struct {
	x, y, width, height int
	layers              []*Layer
	buttonSize          int
	game                *Game
	isDragging          bool
	dragStartX          int
	dragStartY          int
	dragOffsetX         int
	dragOffsetY         int
	visible             bool
	colors              struct {
		background  color.RGBA
		header      color.RGBA
		border      color.RGBA
		text        color.RGBA
		buttonBg    color.RGBA
		selectedRow color.RGBA
		checkmark   color.RGBA
	}
}

func NewLayerPanel(x, y int, layers []*Layer, game *Game) *LayerPanel {
	p := &LayerPanel{
		x:          x,
		y:          y,
		width:      200,
		height:     len(layers)*30 + 40,
		layers:     layers,
		buttonSize: 20,
		visible:    false,
		game:       game,
	}

	// Material theme colors
	p.colors.background = color.RGBA{33, 33, 33, 220}   // Dark grey
	p.colors.header = color.RGBA{25, 25, 25, 255}       // Darker grey
	p.colors.border = color.RGBA{66, 66, 66, 255}       // Light grey
	p.colors.text = color.RGBA{255, 255, 255, 255}      // White
	p.colors.buttonBg = color.RGBA{66, 66, 66, 255}     // Light grey
	p.colors.selectedRow = color.RGBA{33, 150, 243, 64} // Blue-500 with alpha
	p.colors.checkmark = color.RGBA{33, 150, 243, 255}  // Blue-500

	return p
}

func (p *LayerPanel) Update() error {
	if !p.visible {
		return nil
	}

	mouseX, mouseY := ebiten.CursorPosition()

	// Check for close button click
	closeX := p.x + p.width - 25
	closeY := p.y + 5
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if mouseX >= closeX && mouseX <= closeX+20 &&
			mouseY >= closeY && mouseY <= closeY+20 {
			p.visible = false
			return nil
		}
	}

	// Handle dragging
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !p.isDragging {
			// Check if click is on panel header area
			if mouseX >= p.x && mouseX <= p.x+p.width &&
				mouseY >= p.y && mouseY <= p.y+30 {
				p.isDragging = true
				p.dragStartX = mouseX
				p.dragStartY = mouseY
				p.dragOffsetX = mouseX - p.x
				p.dragOffsetY = mouseY - p.y
			}
		} else {
			// Update panel position while dragging
			newX := mouseX - p.dragOffsetX
			newY := mouseY - p.dragOffsetY

			// Keep panel within screen bounds
			screenWidth, screenHeight := p.game.ScreenWidth, p.game.ScreenHeight
			newX = max(0, min(newX, screenWidth-p.width))
			newY = max(0, min(newY, screenHeight-p.height))

			p.x = newX
			p.y = newY
		}
	} else {
		p.isDragging = false
	}

	// Handle clicks when not dragging
	if !p.isDragging && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if mouseX >= p.x && mouseX <= p.x+p.width && mouseY >= p.y+headerHeight {
			row := (mouseY - (p.y + headerHeight)) / layerRowHeight
			if row >= 0 && row < len(p.layers) {
				// Calculate checkbox bounds
				checkboxX := p.x + 10
				checkboxY := p.y + headerHeight + (row * layerRowHeight) + (layerRowHeight-checkboxSize)/2

				// Check if click was in checkbox
				if mouseX >= checkboxX && mouseX <= checkboxX+checkboxSize &&
					mouseY >= checkboxY && mouseY <= checkboxY+checkboxSize {
					// Toggle visibility
					p.layers[row].Visible = !p.layers[row].Visible
					p.game.needRedraw = true
				} else if mouseX >= checkboxX+checkboxSize {
					// Click was on layer name area - select layer
					p.game.currentLayer = p.layers[row]
				}
			}
		}
	}

	return nil
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (p *LayerPanel) Draw(screen *ebiten.Image) {
	if !p.visible {
		return
	}

	// Draw panel background
	vector.DrawFilledRect(screen,
		float32(p.x), float32(p.y),
		float32(p.width), float32(p.height),
		color.RGBA{40, 40, 40, 220}, false)

	// Draw darker header background
	vector.DrawFilledRect(screen,
		float32(p.x), float32(p.y),
		float32(p.width), float32(30),
		color.RGBA{30, 30, 30, 255}, false)

	// Draw panel border
	vector.StrokeRect(screen,
		float32(p.x), float32(p.y),
		float32(p.width), float32(p.height),
		1, color.RGBA{60, 60, 60, 255}, false)

	// Draw close button
	closeX := p.x + p.width - 25
	closeY := p.y + 5
	vector.DrawFilledRect(screen,
		float32(closeX), float32(closeY),
		20, 20,
		color.RGBA{60, 60, 60, 255}, false)

	// Draw X
	vector.StrokeLine(screen,
		float32(closeX+5), float32(closeY+5),
		float32(closeX+15), float32(closeY+15),
		2, color.RGBA{200, 200, 200, 255}, false)
	vector.StrokeLine(screen,
		float32(closeX+15), float32(closeY+5),
		float32(closeX+5), float32(closeY+15),
		2, color.RGBA{200, 200, 200, 255}, false)

	// Draw "Layers" header
	p.game.drawText(screen, float64(p.x+10), float64(p.y+15),
		color.RGBA{200, 200, 200, 255}, "Layers")

	// Draw layer entries
	for i, layer := range p.layers {
		rowY := p.y + headerHeight + (i * layerRowHeight)

		// Draw selection highlight if this is current layer
		if layer == p.game.currentLayer {
			vector.DrawFilledRect(screen,
				float32(p.x), float32(rowY),
				float32(p.width), float32(layerRowHeight),
				p.colors.selectedRow, false)
		}

		// Calculate centered checkbox position
		checkboxX := p.x + 10
		checkboxY := rowY + (layerRowHeight-checkboxSize)/2

		// Draw checkbox background
		vector.DrawFilledRect(screen,
			float32(checkboxX), float32(checkboxY),
			float32(checkboxSize), float32(checkboxSize),
			p.colors.buttonBg, false)

		if layer.Visible {
			// Draw checkmark with adjusted size
			checkStartX := float32(checkboxX + 3)
			checkMiddleX := float32(checkboxX + 6)
			checkEndX := float32(checkboxX + checkboxSize - 3)
			checkStartY := float32(checkboxY + checkboxSize/2)
			checkMiddleY := float32(checkboxY + checkboxSize - 4)
			checkLowestY := float32(checkboxY + 4)

			vector.StrokeLine(screen,
				checkStartX, checkStartY,
				checkMiddleX, checkMiddleY,
				2, p.colors.checkmark, false)
			vector.StrokeLine(screen,
				checkMiddleX, checkMiddleY,
				checkEndX, checkLowestY,
				2, p.colors.checkmark, false)
		}

		// Draw layer name
		p.game.drawText(screen, float64(p.x+40), float64(rowY+layerRowHeight/2),
			p.colors.text, layer.Name)
	}
}

func (p *LayerPanel) UpdateLayers(layers []*Layer) {
	p.layers = layers
	p.height = len(layers)*30 + 40 // Update height to accommodate new layers
}
