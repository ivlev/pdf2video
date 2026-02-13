package analyzer

import "fmt"

// NewDetector creates a detector based on the specified variant
func NewDetector(variant string) (Detector, error) {
	switch variant {
	case "contrast", "":
		return NewContrastDetector(), nil
	case "ocr":
		return nil, fmt.Errorf("OCR detector not yet implemented")
	case "ai":
		return nil, fmt.Errorf("AI detector not yet implemented")
	default:
		return nil, fmt.Errorf("unknown detector variant: %s", variant)
	}
}
