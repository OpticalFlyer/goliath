// layerpanel.go
package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type LayerPanel struct {
	x, y, width, height int
	layers              []*GeometryLayer
	buttonSize          int
	game                *Game
	isDragging          bool
	dragStartX          int
	dragStartY          int
	dragOffsetX         int
	dragOffsetY         int
	visible             bool
}

func NewLayerPanel(x, y int, layers []*GeometryLayer, game *Game) *LayerPanel {
	return &LayerPanel{
		x:          x,
		y:          y,
		width:      200,
		height:     len(layers)*30 + 40,
		layers:     layers,
		buttonSize: 20,
		visible:    false,
		game:       game,
	}
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

	// Only handle visibility toggles when not dragging
	if !p.isDragging && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// Check if click is within panel bounds
		if mouseX >= p.x && mouseX <= p.x+p.width && mouseY >= p.y && mouseY <= p.y+p.height {
			// Calculate which layer row was clicked
			row := (mouseY - p.y - 30) / 30
			if row >= 0 && row < len(p.layers) {
				// Check if click was on visibility toggle button
				buttonX := p.x + 10
				buttonY := p.y + 30 + row*30
				if mouseX >= buttonX && mouseX <= buttonX+p.buttonSize &&
					mouseY >= buttonY && mouseY <= buttonY+p.buttonSize {
					p.layers[row].Visible = !p.layers[row].Visible
					p.game.needRedraw = true
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
		y := p.y + 30 + i*30

		// Draw visibility toggle button
		vector.DrawFilledRect(screen,
			float32(p.x+10), float32(y),
			float32(p.buttonSize), float32(p.buttonSize),
			color.RGBA{60, 60, 60, 255}, false)

		if layer.Visible {
			// Draw checkmark
			vector.StrokeLine(screen,
				float32(p.x+12), float32(y+p.buttonSize/2),
				float32(p.x+16), float32(y+p.buttonSize-4),
				2, color.RGBA{0, 255, 0, 255}, false)
			vector.StrokeLine(screen,
				float32(p.x+16), float32(y+p.buttonSize-4),
				float32(p.x+28), float32(y+4),
				2, color.RGBA{0, 255, 0, 255}, false)
		}

		// Draw layer name
		p.game.drawText(screen, float64(p.x+40), float64(y+15),
			color.RGBA{200, 200, 200, 255}, layer.Name)
	}
}
