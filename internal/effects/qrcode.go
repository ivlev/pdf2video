package effects

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCode creates a QR code with a given URL, having a transparent background
// and black foreground, ensuring the output image is exactly width x height.
func GenerateQRCode(url string, size int, outputDir string) (string, error) {
	q, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("failed to create qr code: %w", err)
	}

	// Disable default white border
	q.DisableBorder = true

	// Ensure transparent background, black foreground
	q.BackgroundColor = color.Transparent
	q.ForegroundColor = color.White

	// Generate base image (might not be exact size if it doesn't divide evenly)
	// Actually we ask for size-40 to leave a 20px padding
	qrSize := size - 40
	if qrSize <= 0 {
		qrSize = size
	}
	img := q.Image(qrSize)
	bounds := img.Bounds()

	// Create a canvas of exact size x size
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))

	// By default NewRGBA has 0-values which is transparent black: RGBA{0,0,0,0}.
	// We can explicitly clear just to be safe.
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.Transparent}, image.Point{}, draw.Src)

	// Center the QR code inside the canvas
	xOff := (size - bounds.Dx()) / 2
	yOff := (size - bounds.Dy()) / 2

	// Using draw.Over over a transparent canvas preserves the transparency of the QR's background
	draw.Draw(canvas, bounds.Add(image.Point{xOff, yOff}), img, bounds.Min, draw.Over)

	outputPath := filepath.Join(outputDir, "overlay_qr.png")
	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create qr file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, canvas); err != nil {
		return "", fmt.Errorf("failed to encode qr file: %w", err)
	}

	return outputPath, nil
}
