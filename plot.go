package main

import (
	"bytes"
	"image/png"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/signintech/gopdf"
)

// plotToPDF creates a PDF from the current screen view
func (g *Game) plotToPDF() error {
	// Create a new temporary image with the same dimensions as the screen
	tempScreen := ebiten.NewImage(g.ScreenWidth, g.ScreenHeight)

	// Draw everything to the temporary image
	// This effectively calls the same Draw function that draws to the screen
	g.Draw(tempScreen)

	// Create a new PDF in landscape mode
	pdf := gopdf.GoPdf{}
	pageSize := gopdf.Rect{W: gopdf.PageSizeA4.H, H: gopdf.PageSizeA4.W} // Swap width and height for landscape
	pdf.Start(gopdf.Config{PageSize: pageSize})
	pdf.AddPage()

	// Convert the temporary image to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, tempScreen); err != nil {
		return err
	}

	// Register the image
	img, err := gopdf.ImageHolderByReader(&buf)
	if err != nil {
		return err
	}

	// Get the actual dimensions of the screen
	imgWidth := float64(g.ScreenWidth)
	imgHeight := float64(g.ScreenHeight)

	// Scale image to fit within page margins while maintaining aspect ratio
	const marginMM float64 = 10 // 10mm margins
	maxWidth := pageSize.W - (2 * marginMM)
	maxHeight := pageSize.H - (2 * marginMM)

	scale := math.Min(maxWidth/imgWidth, maxHeight/imgHeight)
	finalWidth := imgWidth * scale
	finalHeight := imgHeight * scale

	// Calculate position to center the image on the page
	x := (pageSize.W - finalWidth) / 2
	y := (pageSize.H - finalHeight) / 2

	// Draw the image on the page
	pdf.ImageByHolder(img, x, y, &gopdf.Rect{W: finalWidth, H: finalHeight})

	// Save the PDF to a file
	return pdf.WritePdf("plot.pdf")
}
