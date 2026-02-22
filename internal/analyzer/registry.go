package analyzer

import (
	"fmt"
)

// NewDetector creates a detector based on the specified variant
func NewDetector(variant string) (Detector, error) {
	switch variant {
	case "enhanced", "":
		return NewEnhancedDetector(), nil
	case "contrast":
		return NewContrastDetector(), nil
	case "ocr":
		return nil, fmt.Errorf("ocr detector not yet implemented")
	case "ai":
		return nil, fmt.Errorf("ai detector not yet implemented")
	default:
		return nil, fmt.Errorf("unknown detector variant: %s", variant)
	}
}
