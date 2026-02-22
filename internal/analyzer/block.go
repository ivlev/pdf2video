package analyzer

import "image"

// BlockType represents the categorized content of a block
type BlockType string

const (
	BlockTypeText       BlockType = "text"
	BlockTypeImage      BlockType = "image"
	BlockTypeChart      BlockType = "chart"
	BlockTypeDiagram    BlockType = "diagram"
	BlockTypeHeader     BlockType = "header"
	BlockTypeFooter     BlockType = "footer"
	BlockTypeBackground BlockType = "background"
	BlockTypeUnknown    BlockType = "unknown"
)

// BlockMetrics contains quantitative info about a block
type BlockMetrics struct {
	EdgeDensity   float64 // Density of edges (Sobel/Gradient)
	ColorVariance float64 // Variation in pixel colors
	AspectRatio   float64 // Width/Height ratio
	RelativeSize  float64 // Area relative to page area
}

// Block represents a detected region of interest with metadata
type Block struct {
	Rect       image.Rectangle
	Type       BlockType
	Confidence float64
	Score      float64 // Informational importance score (0.0-1.0)
	Density    float64 // Content density
	Priority   float64 // Execution priority

	Metrics BlockMetrics
}
