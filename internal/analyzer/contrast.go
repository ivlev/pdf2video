package analyzer

import (
	"image"
)

// ContrastDetector implements edge-based region detection using Sobel operator
type ContrastDetector struct {
	MinBlockArea  int     // Minimum area in pixels²
	EdgeThreshold float64 // Gradient magnitude threshold
}

// NewContrastDetector creates a new contrast-based detector with default settings
func NewContrastDetector() *ContrastDetector {
	return &ContrastDetector{
		MinBlockArea:  500,  // ~22x22 pixels minimum
		EdgeThreshold: 30.0, // Moderate sensitivity
	}
}

// Detect finds regions of interest using edge detection and morphology
func (d *ContrastDetector) Detect(img image.Image) ([]Block, error) {
	// Step 1: Convert to grayscale
	gray := toGrayscale(img)

	// Step 2: Apply Sobel edge detection
	edges := sobelEdgeDetection(gray, d.EdgeThreshold)

	// Step 3: Morphological dilation to connect nearby edges
	dilated := dilate(edges, 5, 2)

	// Step 4: Find connected components (contours)
	contours := findContours(dilated)

	// Step 5: Filter by minimum area and create Blocks
	blocks := []Block{}
	for _, rect := range contours {
		area := rect.Dx() * rect.Dy()
		if area >= d.MinBlockArea {
			blocks = append(blocks, Block{
				Rect:       rect,
				Type:       BlockTypeUnknown,
				Confidence: 0.7,
			})
		}
	}

	return blocks, nil
}
