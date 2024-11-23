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
	indentSize int // pixels per indent level
	minWidth   int // minimum panel width
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
		indentSize: 20,
		minWidth:   200,
	}

	// Material theme colors
	p.colors.background = color.RGBA{33, 33, 33, 220}   // Dark grey
	p.colors.header = color.RGBA{25, 25, 25, 255}       // Darker grey
	p.colors.border = color.RGBA{66, 66, 66, 255}       // Light grey
	p.colors.text = color.RGBA{255, 255, 255, 255}      // White
	p.colors.buttonBg = color.RGBA{66, 66, 66, 255}     // Light grey
	p.colors.selectedRow = color.RGBA{33, 150, 243, 64} // Blue-500 with alpha
	p.colors.checkmark = color.RGBA{33, 150, 243, 255}  // Blue-500
	p.UpdateLayers(layers)

	return p
}

// Add new method to calculate required width for a layer
func (p *LayerPanel) calculateLayerWidth(layer *Layer, depth int) int {
	// Base width needed for indent + arrow + checkbox + padding
	baseWidth := (depth * p.indentSize) + 16 + checkboxSize + 30

	// Add text width
	textWidth := len(layer.Name) * 8 // Approximate width per character

	return baseWidth + textWidth
}

// Add method to recursively find maximum required width
func (p *LayerPanel) calculateMaxWidth(layers []*Layer, depth int) int {
	maxWidth := 0
	for _, layer := range layers {
		// Calculate width needed for this layer
		layerWidth := p.calculateLayerWidth(layer, depth)
		if layerWidth > maxWidth {
			maxWidth = layerWidth
		}

		// Check children if expanded
		if layer.Expanded && len(layer.Children) > 0 {
			childWidth := p.calculateMaxWidth(layer.Children, depth+1)
			if childWidth > maxWidth {
				maxWidth = childWidth
			}
		}
	}
	return maxWidth
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
			// Convert to panel-relative coordinates
			relX := mouseX - p.x
			relY := mouseY - (p.y + headerHeight)

			// Recursively find and handle clicked layer
			p.handleLayerClick(relX, relY, 0, 0, p.layers)
		}
	}

	return nil
}

func (p *LayerPanel) handleLayerClick(x, y, depth, currentY int, layers []*Layer) bool {
	for _, layer := range layers {
		if y >= currentY && y < currentY+layerRowHeight {
			indentX := depth * p.indentSize
			checkboxX := indentX + 20
			checkboxY := (layerRowHeight - checkboxSize) / 2

			// Check checkbox bounds
			if x >= checkboxX && x <= checkboxX+checkboxSize &&
				(y-currentY) >= checkboxY && (y-currentY) <= checkboxY+checkboxSize {
				layer.Visible = !layer.Visible
				p.game.needRedraw = true
				return true
			}

			// Check layer name area
			if x >= checkboxX+checkboxSize {
				p.game.currentLayer = layer
				p.game.needRedraw = true
				return true
			}

			// Check expand/collapse arrow
			if len(layer.Children) > 0 && x >= indentX && x < indentX+16 {
				layer.Expanded = !layer.Expanded
				p.height = p.countVisibleRows(p.layers)*layerRowHeight + headerHeight + 10
				return true
			}

			return true
		}

		currentY += layerRowHeight
		if layer.Expanded && len(layer.Children) > 0 {
			if p.handleLayerClick(x, y, depth+1, currentY, layer.Children) {
				return true
			}
			currentY += p.countVisibleRows(layer.Children) * layerRowHeight
		}
	}
	return false
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

// Helper to count total visible rows
func (p *LayerPanel) countVisibleRows(layers []*Layer) int {
	count := 0
	for _, layer := range layers {
		count++ // Count this layer
		if layer.Expanded {
			count += p.countVisibleRows(layer.Children)
		}
	}
	return count
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

	// Draw layers recursively
	yOffset := 0
	for _, layer := range p.layers {
		p.drawLayerRecursive(screen, layer, 0, &yOffset)
	}
}

func (p *LayerPanel) drawLayerRecursive(screen *ebiten.Image, layer *Layer, depth int, yOffset *int) {
	rowY := p.y + headerHeight + *yOffset
	indentX := p.x + (depth * p.indentSize)

	// Draw selection highlight
	if layer == p.game.currentLayer {
		vector.DrawFilledRect(screen,
			float32(p.x), float32(rowY),
			float32(p.width), float32(layerRowHeight),
			p.colors.selectedRow, false)
	}

	// Draw expand/collapse arrow if has children
	if len(layer.Children) > 0 {
		arrowX := float32(indentX + 2)
		arrowY := float32(rowY + layerRowHeight/2)
		if layer.Expanded {
			// Draw down arrow
			vector.StrokeLine(screen, arrowX, arrowY-4, arrowX+8, arrowY+4, 2, p.colors.text, false)
			vector.StrokeLine(screen, arrowX+8, arrowY+4, arrowX+16, arrowY-4, 2, p.colors.text, false)
		} else {
			// Draw right arrow
			vector.StrokeLine(screen, arrowX, arrowY-4, arrowX+8, arrowY+4, 2, p.colors.text, false)
			vector.StrokeLine(screen, arrowX, arrowY+12, arrowX+8, arrowY+4, 2, p.colors.text, false)
		}
	}

	// Draw checkbox
	checkboxX := indentX + 20
	checkboxY := rowY + (layerRowHeight-checkboxSize)/2
	vector.DrawFilledRect(screen,
		float32(checkboxX), float32(checkboxY),
		float32(checkboxSize), float32(checkboxSize),
		p.colors.buttonBg, false)

	if layer.Visible {
		drawCheckmark(screen, checkboxX, checkboxY, p.colors.checkmark)
	}

	// Draw layer name
	p.game.drawText(screen, float64(checkboxX+checkboxSize+5), float64(rowY+layerRowHeight/2),
		p.colors.text, layer.Name)

	*yOffset += layerRowHeight

	// Draw children if expanded
	if layer.Expanded {
		for _, child := range layer.Children {
			p.drawLayerRecursive(screen, child, depth+1, yOffset)
		}
	}
}

func drawCheckmark(screen *ebiten.Image, x, y int, col color.RGBA) {
	// Draw checkmark with two strokes
	// First stroke (down-right part)
	vector.StrokeLine(screen,
		float32(x+3), float32(y+8),
		float32(x+6), float32(y+11),
		2, col, false)

	// Second stroke (up-right part)
	vector.StrokeLine(screen,
		float32(x+6), float32(y+11),
		float32(x+13), float32(y+4),
		2, col, false)
}

// Update UpdateLayers to adjust width
func (p *LayerPanel) UpdateLayers(layers []*Layer) {
	p.layers = layers

	// Update height
	totalRows := p.countVisibleRows(layers)
	p.height = totalRows*layerRowHeight + headerHeight + 10

	// Update width
	requiredWidth := p.calculateMaxWidth(layers, 0)
	p.width = max(p.minWidth, requiredWidth)

	// Keep panel within screen bounds if needed
	if p.x+p.width > p.game.ScreenWidth {
		p.x = max(0, p.game.ScreenWidth-p.width)
	}
}
