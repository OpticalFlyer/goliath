package ui

import "github.com/hajimehoshi/ebiten/v2"

// Component represents the basic building block of the UI system.
// All UI elements must implement this interface.
type Component interface {
	Update() error
	Draw(screen *ebiten.Image)
	Bounds() Rectangle
	HandleInput(x, y float64, pressed bool) bool
	SetParent(parent Container)
	GetParent() Container
}

// Container represents a Component that can hold and manage other Components.
type Container interface {
	Component
	AddChild(child Component)
	RemoveChild(child Component)
	Children() []Component
	Layout() Layout
}

// Rectangle represents the bounds of a Component
type Rectangle struct {
	X, Y          float64
	Width, Height float64
}

// Layout defines how Components are arranged within a Container
type Layout interface {
	ArrangeChildren(container Container)
}
