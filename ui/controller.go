package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Controller is the root container for the UI system
type Controller struct {
	children  []Component
	bounds    Rectangle
	debugMode bool
}

// Ensure Controller implements both Container and Component
var _ Container = (*Controller)(nil)
var _ Component = (*Controller)(nil)

func (c *Controller) SetParent(parent Container) {
	// Controller is root, ignore parent setting
}

func (c *Controller) GetParent() Container {
	// Controller has no parent, return nil
	return nil
}

func NewController() *Controller {
	return &Controller{
		children: make([]Component, 0),
		bounds:   Rectangle{0, 0, 800, 600}, // default size
	}
}

// Container interface implementation
func (c *Controller) AddChild(child Component) {
	c.children = append(c.children, child)
	child.SetParent(c)
}

func (c *Controller) RemoveChild(child Component) {
	for i, comp := range c.children {
		if comp == child {
			c.children = append(c.children[:i], c.children[i+1:]...)
			break
		}
	}
}

func (c *Controller) Children() []Component {
	return c.children
}

func (c *Controller) Layout() Layout {
	return nil // Root container doesn't need a layout
}

// Component interface implementation
func (c *Controller) Update() error {
	for _, child := range c.children {
		if err := child.Update(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) Draw(screen *ebiten.Image) {
	for _, child := range c.children {
		child.Draw(screen)
	}

	if c.debugMode {
		c.drawDebugInfo(screen)
	}
}

func (c *Controller) Bounds() Rectangle {
	return c.bounds
}

func (c *Controller) HandleInput(x, y float64, pressed bool) bool {
	// Handle input in reverse order (top-most first)
	for i := len(c.children) - 1; i >= 0; i-- {
		if c.children[i].HandleInput(x, y, pressed) {
			return true
		}
	}
	return false
}

// Controller-specific methods
func (c *Controller) SetDebugMode(enabled bool) {
	c.debugMode = enabled
}

func (c *Controller) UpdateWindowSize(width, height int) {
	c.bounds = Rectangle{0, 0, float64(width), float64(height)}
	// Update any child components that need window dimensions
	for _, child := range c.children {
		if p, ok := child.(*Panel); ok {
			p.UpdateWindowSize(width, height)
		}
	}
}

func (c *Controller) drawDebugInfo(screen *ebiten.Image) {
	fps := ebiten.ActualFPS()
	tps := ebiten.ActualTPS()
	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %.2f TPS: %.2f", fps, tps))
}

func (c *Controller) IsInteractingWithUI() bool {
	// Check mouse interaction
	x, y := ebiten.CursorPosition()
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		return c.HandleInput(float64(x), float64(y), true)
	}

	// Check touch interaction
	touches := make([]ebiten.TouchID, 0, 8)
	touches = ebiten.AppendTouchIDs(touches)
	for _, id := range touches {
		x, y := ebiten.TouchPosition(id)
		if c.HandleInput(float64(x), float64(y), true) {
			return true
		}
	}

	return false
}
