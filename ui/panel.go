package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type DockState int

const (
	dockNone DockState = iota
	dockLeft
	dockRight
	dockTop
	dockBottom
)

type resizableSides struct {
	left   bool
	right  bool
	top    bool
	bottom bool
}

const (
	titleBarHeight = 20.0
	resizeArea     = 5.0
	minPanelWidth  = 100.0
	minPanelHeight = 50.0
	previewAlpha   = 84
	panelAlpha     = 200
)

type ResizeState int

const (
	resizeNone ResizeState = iota
	resizeLeft
	resizeRight
	resizeTop
	resizeBottom
	resizeTopLeft
	resizeTopRight
	resizeBottomLeft
	resizeBottomRight
)

type Panel struct {
	X, Y          float64
	Width, Height float64
	Title         string

	// Docking state
	dockState     DockState
	isDockPreview bool

	// Undocked dimensions (saved before docking)
	undockedX, undockedY          float64
	undockedWidth, undockedHeight float64

	// Interaction state
	isDragging                bool
	isResizing                bool
	dragStartX                float64
	dragStartY                float64
	resizeState               ResizeState
	startWidth                float64
	startHeight               float64
	mouseButtonPreviouslyDown bool
	initialClickX             float64
	initialClickY             float64

	// Window dimensions
	windowWidth  int
	windowHeight int

	resizableSides resizableSides
}

func NewPanel(x, y, width, height float64, title string) *Panel {
	return &Panel{
		X:              x,
		Y:              y,
		Width:          width,
		Height:         height,
		Title:          title,
		dockState:      dockNone,
		undockedX:      x,
		undockedY:      y,
		undockedWidth:  width,
		undockedHeight: height,
		windowWidth:    800, // Default window size
		windowHeight:   600, // Default window size
		resizableSides: resizableSides{
			left:   true,
			right:  true,
			top:    true,
			bottom: true,
		},
	}
}

func (p *Panel) checkDocking(x, y float64) {
	prevDockState := p.dockState
	dockThreshold := 20.0

	// Remember current drag position
	currentX := p.X
	currentY := p.Y

	if x < dockThreshold {
		p.dockState = dockLeft
	} else if float64(p.windowWidth)-x < dockThreshold {
		p.dockState = dockRight
	} else if y < dockThreshold {
		p.dockState = dockTop
	} else if float64(p.windowHeight)-y < dockThreshold {
		p.dockState = dockBottom
	} else {
		p.dockState = dockNone
		if p.isDockPreview {
			// Restore to current drag position
			p.X = currentX
			p.Y = currentY
			p.Width = p.undockedWidth
			p.Height = p.undockedHeight
		}
		p.isDockPreview = false
		return
	}

	// Save undocked dimensions before preview if not already in preview
	if prevDockState == dockNone && !p.isDockPreview {
		p.undockedWidth = p.Width
		p.undockedHeight = p.Height
		p.undockedX = p.X
		p.undockedY = p.Y
	}

	p.isDockPreview = true
	p.UpdateWindowSize(p.windowWidth, p.windowHeight)
}

func (p *Panel) UpdateWindowSize(width, height int) {
	p.windowWidth = width
	p.windowHeight = height

	// Update panel dimensions if docked
	switch p.dockState {
	case dockLeft:
		p.X = 0
		p.Y = 0
		// Only set Width if not already docked
		if !p.isDockPreview && p.dockState != dockLeft {
			p.Width = 200
		}
		p.Height = float64(height)
	case dockRight:
		p.Y = 0
		p.Height = float64(height)
		// Only set Width if not already docked
		if !p.isDockPreview && p.dockState != dockRight {
			p.Width = 200
		}
		p.X = float64(width) - p.Width
	case dockTop:
		p.X = 0
		p.Y = 0
		p.Width = float64(width)
		// Only set Height if not already docked
		if !p.isDockPreview && p.dockState != dockTop {
			p.Height = 200
		}
	case dockBottom:
		p.X = 0
		p.Width = float64(width)
		// Only set Height if not already docked
		if !p.isDockPreview && p.dockState != dockBottom {
			p.Height = 200
		}
		p.Y = float64(height) - p.Height
	}
	p.updateResizableSides()
}

func (p *Panel) getResizeArea(x, y float64) ResizeState {
	// Adjust hit testing based on dock state
	var left, right, top, bottom bool

	switch p.dockState {
	case dockLeft:
		// When docked left, only right edge is resizable
		right = x >= p.X+p.Width-resizeArea && x <= p.X+p.Width+resizeArea
	case dockRight:
		// When docked right, only left edge is resizable
		left = x >= p.X-resizeArea && x <= p.X+resizeArea
	case dockTop:
		// When docked top, only bottom edge is resizable
		bottom = y >= p.Y+p.Height-resizeArea && y <= p.Y+p.Height+resizeArea
	case dockBottom:
		// When docked bottom, only top edge is resizable
		top = y >= p.Y-resizeArea && y <= p.Y+resizeArea
	case dockNone:
		// Normal hit testing when undocked
		left = x >= p.X-resizeArea && x <= p.X+resizeArea && p.resizableSides.left
		right = x >= p.X+p.Width-resizeArea && x <= p.X+p.Width+resizeArea && p.resizableSides.right
		top = y >= p.Y-resizeArea && y <= p.Y+resizeArea && p.resizableSides.top
		bottom = y >= p.Y+p.Height-resizeArea && y <= p.Y+p.Height+resizeArea && p.resizableSides.bottom
	}

	// Return appropriate resize state
	if p.dockState != dockNone {
		// When docked, only return the single valid resize edge
		if left {
			return resizeLeft
		} else if right {
			return resizeRight
		} else if top {
			return resizeTop
		} else if bottom {
			return resizeBottom
		}
	} else {
		// Normal resize state handling for undocked state
		if left && top {
			return resizeTopLeft
		} else if right && top {
			return resizeTopRight
		} else if left && bottom {
			return resizeBottomLeft
		} else if right && bottom {
			return resizeBottomRight
		} else if left {
			return resizeLeft
		} else if right {
			return resizeRight
		} else if top {
			return resizeTop
		} else if bottom {
			return resizeBottom
		}
	}

	return resizeNone
}

func (p *Panel) updateCursor() {
	x, y := ebiten.CursorPosition()
	resizeState := p.getResizeArea(float64(x), float64(y))

	switch resizeState {
	case resizeLeft, resizeRight:
		ebiten.SetCursorShape(ebiten.CursorShapeEWResize)
	case resizeTop, resizeBottom:
		ebiten.SetCursorShape(ebiten.CursorShapeNSResize)
	case resizeTopLeft, resizeBottomRight:
		ebiten.SetCursorShape(ebiten.CursorShapeNWSEResize)
	case resizeTopRight, resizeBottomLeft:
		ebiten.SetCursorShape(ebiten.CursorShapeNESWResize)
	default:
		if p.isInTitleBar(float64(x), float64(y)) {
			ebiten.SetCursorShape(ebiten.CursorShapeMove)
		} else {
			ebiten.SetCursorShape(ebiten.CursorShapeDefault)
		}
	}
}

func (p *Panel) Update() error {
	x, y := ebiten.CursorPosition()
	fx, fy := float64(x), float64(y)

	p.updateCursor()
	isMousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	if isMousePressed {
		if !p.mouseButtonPreviouslyDown {
			p.initialClickX = fx
			p.initialClickY = fy
			p.mouseButtonPreviouslyDown = true

			if p.isInTitleBar(p.initialClickX, p.initialClickY) {
				p.isDragging = true

				if p.dockState == dockNone {
					// Save current undocked state before potential docking
					p.undockedWidth = p.Width
					p.undockedHeight = p.Height
					p.undockedX = p.X
					p.undockedY = p.Y
				} else {
					// Undocking - restore previous undocked dimensions
					relativeX := (fx - p.X) / p.Width
					p.dockState = dockNone
					p.isDockPreview = false
					p.Width = p.undockedWidth
					p.Height = p.undockedHeight
					p.X = fx - (p.Width * relativeX)
					p.Y = fy - titleBarHeight/2
					p.updateResizableSides()
				}

				p.dragStartX = fx - p.X
				p.dragStartY = fy - p.Y
			} else {
				resizeState := p.getResizeArea(p.initialClickX, p.initialClickY)
				if resizeState != resizeNone {
					p.isResizing = true
					p.resizeState = resizeState
					p.dragStartX = p.initialClickX
					p.dragStartY = p.initialClickY
					p.startWidth = p.Width
					p.startHeight = p.Height
				}
			}
		}

		if p.isDragging {
			p.X = fx - p.dragStartX
			p.Y = fy - p.dragStartY
			p.checkDocking(fx, fy)
		} else if p.isResizing {
			deltaX := fx - p.dragStartX
			deltaY := fy - p.dragStartY

			switch p.resizeState {
			case resizeLeft:
				if p.dockState == dockRight {
					// When docked right, adjust width from left edge
					newWidth := max(minPanelWidth, p.startWidth-deltaX)
					p.X = float64(p.windowWidth) - newWidth
					p.Width = newWidth
				} else if p.dockState == dockNone {
					p.Width = max(minPanelWidth, p.startWidth-deltaX)
					p.X = p.dragStartX + deltaX
				}
			case resizeRight:
				if p.dockState == dockLeft {
					// When docked left, just adjust width
					p.Width = max(minPanelWidth, p.startWidth+deltaX)
				} else if p.dockState == dockNone {
					p.Width = max(minPanelWidth, p.startWidth+deltaX)
				}
			case resizeTop:
				if p.dockState == dockBottom {
					// When docked bottom, adjust height from top edge
					newHeight := max(minPanelHeight, p.startHeight-deltaY)
					p.Y = float64(p.windowHeight) - newHeight
					p.Height = newHeight
				} else if p.dockState == dockNone {
					p.Height = max(minPanelHeight, p.startHeight-deltaY)
					p.Y = p.dragStartY + deltaY
				}
			case resizeBottom:
				if p.dockState == dockTop {
					// When docked top, just adjust height
					p.Height = max(minPanelHeight, p.startHeight+deltaY)
				} else if p.dockState == dockNone {
					p.Height = max(minPanelHeight, p.startHeight+deltaY)
				}
			case resizeTopLeft:
				p.Width = max(minPanelWidth, p.startWidth-deltaX)
				p.Height = max(minPanelHeight, p.startHeight-deltaY)
				p.X = p.dragStartX + deltaX
				p.Y = p.dragStartY + deltaY
			case resizeTopRight:
				p.Width = max(minPanelWidth, p.startWidth+deltaX)
				p.Height = max(minPanelHeight, p.startHeight-deltaY)
				p.Y = p.dragStartY + deltaY
			case resizeBottomLeft:
				p.Width = max(minPanelWidth, p.startWidth-deltaX)
				p.Height = max(minPanelHeight, p.startHeight+deltaY)
				p.X = p.dragStartX + deltaX
			case resizeBottomRight:
				p.Width = max(minPanelWidth, p.startWidth+deltaX)
				p.Height = max(minPanelHeight, p.startHeight+deltaY)
			}

			// If not docked, save current dimensions as undocked state
			if p.dockState == dockNone {
				p.undockedWidth = p.Width
				p.undockedHeight = p.Height
				p.undockedX = p.X
				p.undockedY = p.Y
			}
		}
	} else {
		if p.isDragging && p.isDockPreview {
			p.isDockPreview = false
			if p.dockState != dockNone {
				p.UpdateWindowSize(p.windowWidth, p.windowHeight)
			}
		}
		p.isDragging = false
		p.isResizing = false
		p.mouseButtonPreviouslyDown = false
	}

	return nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (p *Panel) Draw(screen *ebiten.Image) {
	// Set colors based on preview state
	var bgColor, titleColor color.RGBA
	if p.isDockPreview {
		bgColor = color.RGBA{33, 150, 243, previewAlpha}
		titleColor = color.RGBA{60, 60, 60, previewAlpha}
	} else {
		bgColor = color.RGBA{100, 100, 100, panelAlpha}
		titleColor = color.RGBA{60, 60, 60, panelAlpha}
	}

	// Draw panel background
	vector.DrawFilledRect(screen, float32(p.X), float32(p.Y), float32(p.Width), float32(p.Height), bgColor, true)

	// Draw title bar
	vector.DrawFilledRect(screen, float32(p.X), float32(p.Y), float32(p.Width), float32(titleBarHeight), titleColor, true)
}

func (p *Panel) isInTitleBar(x, y float64) bool {
	// Don't capture title bar events if in resize area
	if p.getResizeArea(x, y) != resizeNone {
		return false
	}
	return x >= p.X && x <= p.X+p.Width &&
		y >= p.Y && y <= p.Y+titleBarHeight
}

func (p *Panel) updateResizableSides() {
	p.resizableSides = resizableSides{
		left:   p.dockState != dockRight,
		right:  p.dockState != dockLeft,
		top:    p.dockState != dockBottom,
		bottom: p.dockState != dockTop,
	}
}
