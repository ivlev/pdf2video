package analyzer

import (
	"image"
	"image/color"
	"testing"
)

func TestEnhancedDetector_Detect(t *testing.T) {
	// Create a test image with white background
	img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1000; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Add a "text-like" block (high edge density)
	for y := 100; y < 200; y += 4 {
		for x := 100; x < 400; x += 4 {
			img.Set(x, y, color.Black)
		}
	}

	// Add an "image-like" block (high color variance)
	for y := 300; y < 500; y++ {
		for x := 500; x < 700; x++ {
			c := color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255}
			img.Set(x, y, c)
		}
	}

	detector := NewEnhancedDetector()
	detector.MinScoreThreshold = 0.2

	blocks, err := detector.Detect(img)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should find at least 2 blocks
	if len(blocks) < 2 {
		t.Errorf("Expected at least 2 blocks, got %d", len(blocks))
	}

	foundText := false
	foundImage := false
	for _, b := range blocks {
		if b.Type == BlockTypeText {
			foundText = true
		}
		if b.Type == BlockTypeImage {
			foundImage = true
		}
	}

	if !foundText {
		t.Error("Did not detect text block")
	}
	if !foundImage {
		t.Error("Did not detect image block")
	}
}

func TestEnhancedDetector_Filtering(t *testing.T) {
	// Create a mostly empty image
	img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1000; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Add a very small noisy block that should be filtered
	img.Set(500, 400, color.Black)
	img.Set(501, 400, color.Black)

	detector := NewEnhancedDetector()
	detector.MinBlockArea = 500

	blocks, _ := detector.Detect(img)

	if len(blocks) > 0 {
		t.Errorf("Expected 0 blocks after filtering, got %d", len(blocks))
	}
}
