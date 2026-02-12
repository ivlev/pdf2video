package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/ivlev/pdf2video/internal/analyzer"
	"github.com/ivlev/pdf2video/internal/director"
)

func main() {
	testImagePath := "/tmp/test_slide.png"
	scenarioPath := director.GenerateScenarioPath()

	fmt.Println("=== Smart Zoom Scenario Generation Test ===")
	fmt.Printf("Output: %s\n\n", scenarioPath)

	// Step 1: Create synthetic test image
	fmt.Println("[1/3] Creating synthetic test image...")
	img := createTestImage(1920, 1080)

	f, err := os.Create(testImagePath)
	if err != nil {
		log.Fatalf("Failed to create image file: %v", err)
	}
	defer f.Close()

	err = png.Encode(f, img)
	if err != nil {
		log.Fatalf("Failed to encode image: %v", err)
	}
	fmt.Printf("✓ Created test image: %s (1920x1080)\n\n", testImagePath)

	// Step 2: Analyze image to detect blocks
	fmt.Println("[2/3] Analyzing image for regions of interest...")
	detector, err := analyzer.NewDetector("contrast")
	if err != nil {
		log.Fatalf("Failed to create detector: %v", err)
	}

	blocks, err := detector.Detect(img)
	if err != nil {
		log.Fatalf("Failed to detect blocks: %v", err)
	}
	fmt.Printf("✓ Detected %d blocks\n", len(blocks))
	for i, block := range blocks {
		fmt.Printf("  Block %d: %v (confidence: %.2f)\n", i+1, block.Rect, block.Confidence)
	}
	fmt.Println()

	// Step 3: Generate scenario
	fmt.Println("[3/3] Generating YAML scenario...")
	d := director.NewDirector(img.Bounds().Dx(), img.Bounds().Dy())
	scenario, err := d.GenerateScenario(blocks, "test_slide.png", 15.0)
	if err != nil {
		log.Fatalf("Failed to generate scenario: %v", err)
	}

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(scenarioPath), 0755)

	err = director.WriteScenario(scenario, scenarioPath)
	if err != nil {
		log.Fatalf("Failed to write scenario: %v", err)
	}
	fmt.Printf("✓ Scenario saved to: %s\n\n", scenarioPath)

	// Display scenario summary
	fmt.Println("=== Scenario Summary ===")
	fmt.Printf("Version: %s\n", scenario.Version)
	fmt.Printf("Slides: %d\n", len(scenario.Slides))
	if len(scenario.Slides) > 0 {
		slide := scenario.Slides[0]
		fmt.Printf("Duration: %.1fs\n", slide.Duration)
		fmt.Printf("Keyframes: %d\n", len(slide.Keyframes))
		fmt.Println("\nKeyframes:")
		for i, kf := range slide.Keyframes {
			fmt.Printf("  %d. t=%.1fs, focus=%s, zoom=%.2fx, rect=%dx%d\n",
				i+1, kf.Time, kf.Focus, kf.Zoom, kf.Rect.W, kf.Rect.H)
		}
	}

	fmt.Println("\n✅ Test completed successfully!")
	fmt.Printf("📄 View scenario: cat %s\n", scenarioPath)
}

// createTestImage creates a synthetic slide with text-like blocks
func createTestImage(width, height int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))

	// Fill with light gray background
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetGray(x, y, color.Gray{Y: 240})
		}
	}

	// Draw "title" block (top)
	drawRect(img, 200, 100, 1520, 250, color.Gray{Y: 50})

	// Draw "subtitle" block
	drawRect(img, 200, 300, 1520, 380, color.Gray{Y: 80})

	// Draw "content" blocks
	drawRect(img, 200, 450, 900, 700, color.Gray{Y: 60})
	drawRect(img, 1020, 450, 1720, 700, color.Gray{Y: 60})

	// Draw "footer" block
	drawRect(img, 200, 950, 1720, 1020, color.Gray{Y: 100})

	return img
}

// drawRect draws a filled rectangle
func drawRect(img *image.Gray, x1, y1, x2, y2 int, c color.Gray) {
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.SetGray(x, y, c)
			}
		}
	}
}
