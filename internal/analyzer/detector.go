package analyzer

import "image"

// Detector is the interface for image analysis strategies
type Detector interface {
	Detect(img image.Image) ([]Block, error)
}
