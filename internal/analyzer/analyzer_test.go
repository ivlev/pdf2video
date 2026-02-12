package analyzer

import (
	"image"
	"image/color"
	"testing"
)

func TestContrastDetector(t *testing.T) {
	// Create a simple test image with a white rectangle on black background
	img := image.NewGray(image.Rect(0, 0, 200, 200))

	// Fill with black
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.SetGray(x, y, color.Gray{Y: 0})
		}
	}

	// Draw a white rectangle (simulating text block)
	for y := 50; y < 150; y++ {
		for x := 50; x < 150; x++ {
			img.SetGray(x, y, color.Gray{Y: 255})
		}
	}

	// Test detector
	detector := NewContrastDetector()
	blocks, err := detector.Detect(img)

	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("Expected at least one block, got none")
	}

	// Verify the detected block roughly matches our white rectangle
	block := blocks[0]
	if block.Rect.Dx() < 80 || block.Rect.Dy() < 80 {
		t.Errorf("Block too small: %v", block.Rect)
	}

	t.Logf("Detected %d blocks", len(blocks))
	for i, b := range blocks {
		t.Logf("Block %d: %v (type: %s, confidence: %.2f)", i, b.Rect, b.Type, b.Confidence)
	}
}

func TestDetectorRegistry(t *testing.T) {
	tests := []struct {
		variant string
		wantErr bool
	}{
		{"contrast", false},
		{"", false}, // default
		{"ocr", true},
		{"ai", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			detector, err := NewDetector(tt.variant)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if detector == nil {
					t.Error("Expected detector, got nil")
				}
			}
		})
	}
}
