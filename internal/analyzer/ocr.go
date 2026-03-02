package analyzer

import (
	"image"
)

// OCRDetector is a specialized detector that uses the Source's structured text extraction
// capabilities (like MuPDF/fitz) instead of pure computer vision.
type OCRDetector struct {
	Source interface {
		GetTextBlocks(index int) ([]Block, error)
	}
	PageIndex int
}

func NewOCRDetector(source interface {
	GetTextBlocks(index int) ([]Block, error)
}, pageIndex int) *OCRDetector {
	return &OCRDetector{
		Source:    source,
		PageIndex: pageIndex,
	}
}

func (d *OCRDetector) Detect(img image.Image) ([]Block, error) {
	if d.Source == nil {
		return []Block{}, nil
	}
	return d.Source.GetTextBlocks(d.PageIndex)
}
