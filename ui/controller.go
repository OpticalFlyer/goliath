package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Controller manages all UI elements
type Controller struct {
	panels []*Panel
}

// NewController creates a new UI controller
func NewController() *Controller {
	return &Controller{
		panels: make([]*Panel, 0),
	}
}

// AddPanel adds a new panel to the UI
func (c *Controller) AddPanel(panel *Panel) {
	c.panels = append(c.panels, panel)
}

// Update updates all UI elements
func (c *Controller) Update() error {
	for _, panel := range c.panels {
		if err := panel.Update(); err != nil {
			return err
		}
	}
	return nil
}

// Draw draws all UI elements
func (c *Controller) Draw(screen *ebiten.Image) {
	for _, panel := range c.panels {
		panel.Draw(screen)
	}
}

// UpdateWindowSize updates the window size for all panels
func (c *Controller) UpdateWindowSize(width, height int) {
	for _, panel := range c.panels {
		panel.UpdateWindowSize(width, height)
	}
}

// ShowDebugInfo draws debug information
func (c *Controller) ShowDebugInfo(screen *ebiten.Image) {
	fps := ebiten.ActualFPS()
	tps := ebiten.ActualTPS()
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %.2f TPS: %.2f", fps, tps))
}

// IsInteractingWithUI returns true if any UI element is being interacted with
func (c *Controller) IsInteractingWithUI() bool {
	for _, panel := range c.panels {
		if panel.isDragging || panel.isResizing {
			return true
		}
	}
	return false
}
