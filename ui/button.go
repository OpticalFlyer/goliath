package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var _ Component = (*Button)(nil)

type Button struct {
	x, y          float64
	width, height float64
	text          string
	onClick       func()
	parent        Container

	// State
	isHovered bool
	isPressed bool
}

func (b *Button) SetParent(parent Container) {
	b.parent = parent
}

func (b *Button) GetParent() Container {
	return b.parent
}

func NewButton(x, y float64, text string, onClick func()) *Button {
	return &Button{
		x:       x,
		y:       y,
		width:   100,
		height:  30,
		text:    text,
		onClick: onClick,
	}
}

func (b *Button) Update() error {
	return nil
}

func (b *Button) Draw(screen *ebiten.Image) {
	// Colors
	var bgColor color.Color
	if b.isPressed {
		bgColor = color.RGBA{100, 100, 100, 255}
	} else if b.isHovered {
		bgColor = color.RGBA{180, 180, 180, 255}
	} else {
		bgColor = color.RGBA{150, 150, 150, 255}
	}

	parentBounds := b.parent.Bounds()
	absoluteX := b.x + parentBounds.X
	absoluteY := b.y + parentBounds.Y

	// Draw background
	vector.DrawFilledRect(screen, float32(absoluteX), float32(absoluteY),
		float32(b.width), float32(b.height), bgColor, true)

	// Draw border
	vector.StrokeRect(screen, float32(absoluteX), float32(absoluteY),
		float32(b.width), float32(b.height), 1, color.Black, true)
}

func (b *Button) HandleInput(x, y float64, pressed bool) bool {
	// Check if point is within button bounds
	if x >= b.x && x <= b.x+b.width &&
		y >= b.y && y <= b.y+b.height {
		b.isHovered = true

		if pressed {
			b.isPressed = true
		} else if b.isPressed {
			b.isPressed = false
			if b.onClick != nil {
				b.onClick()
			}
		}
		return true
	}

	b.isHovered = false
	b.isPressed = false
	return false
}

func (b *Button) Bounds() Rectangle {
	return Rectangle{
		X:      b.x,
		Y:      b.y,
		Width:  b.width,
		Height: b.height,
	}
}
