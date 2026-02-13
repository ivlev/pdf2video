package analyzer

import "image"

// Block represents a detected region of interest in an image
type Block struct {
	Rect       image.Rectangle
	Type       string  // "text", "header", "image", "unknown"
	Confidence float64 // 0.0-1.0
}

// Detector is the interface for image analysis strategies
type Detector interface {
	Detect(img image.Image) ([]Block, error)
}
