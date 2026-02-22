package analyzer

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// EnhancedDetector combines several methods of detection and scoring
type EnhancedDetector struct {
	EdgeThreshold     float64
	MinBlockArea      int
	MaxBlocks         int
	MinScoreThreshold float64

	// Weights for scoring
	EdgeWeight          float64
	ColorVarianceWeight float64
	SizeWeight          float64
	PositionWeight      float64

	prioritizer *BlockPrioritizer
}

func NewEnhancedDetector() *EnhancedDetector {
	return &EnhancedDetector{
		EdgeThreshold:     30.0,
		MinBlockArea:      500,
		MaxBlocks:         10,
		MinScoreThreshold: 0.3,

		EdgeWeight:          0.35,
		ColorVarianceWeight: 0.25,
		SizeWeight:          0.20,
		PositionWeight:      0.20,

		prioritizer: NewBlockPrioritizer(),
	}
}

func (d *EnhancedDetector) Detect(img image.Image) ([]Block, error) {
	bounds := img.Bounds()
	gray := toGrayscale(img)
	edges := sobelEdgeDetection(gray, d.EdgeThreshold)
	dilated := dilate(edges, 5, 2)
	contours := findContours(dilated)

	blocks := make([]Block, 0, len(contours))
	for _, rect := range contours {
		area := rect.Dx() * rect.Dy()
		if area < d.MinBlockArea {
			continue
		}

		block := d.analyzeBlock(img, rect, bounds)
		if block.Score >= d.MinScoreThreshold {
			blocks = append(blocks, block)
		}
	}

	// Sort by score
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Score > blocks[j].Score
	})

	// Semantic Prioritization
	blocks = d.prioritizer.Prioritize(blocks, bounds.Dx(), bounds.Dy())

	if len(blocks) > d.MaxBlocks {
		blocks = blocks[:d.MaxBlocks]
	}

	return blocks, nil
}

func (d *EnhancedDetector) analyzeBlock(img image.Image, rect image.Rectangle, pageBounds image.Rectangle) Block {
	metrics := d.calculateMetrics(img, rect, pageBounds)

	// Normalized scores (0.0 - 1.0)
	edgeScore := math.Min(metrics.EdgeDensity/0.5, 1.0)
	colorScore := math.Min(metrics.ColorVariance/5000, 1.0)
	sizeScore := metrics.RelativeSize

	centerX := float64(rect.Min.X+rect.Max.X) / 2 / float64(pageBounds.Dx())
	centerY := float64(rect.Min.Y+rect.Max.Y) / 2 / float64(pageBounds.Dy())
	positionScore := 1.0 - math.Abs(centerX-0.5) - math.Abs(centerY-0.5)
	positionScore = math.Max(0, positionScore)

	totalScore := edgeScore*d.EdgeWeight +
		colorScore*d.ColorVarianceWeight +
		sizeScore*d.SizeWeight +
		positionScore*d.PositionWeight

	blockType := d.classifyBlock(metrics, rect, pageBounds)

	return Block{
		Rect:       rect,
		Type:       blockType,
		Confidence: 0.8,
		Score:      totalScore,
		Density:    metrics.EdgeDensity,
		Metrics:    metrics,
	}
}

func (d *EnhancedDetector) calculateMetrics(img image.Image, rect image.Rectangle, pageBounds image.Rectangle) BlockMetrics {
	var edgeCount int
	var colorSumR, colorSumG, colorSumB float64
	var colorCount int

	bounds := img.Bounds()
	// Use a small step for performance but enough to see edges
	for y := rect.Min.Y; y < rect.Max.Y && y < bounds.Max.Y; y++ {
		var prevGray float64 = -1
		for x := rect.Min.X; x < rect.Max.X && x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			colorSumR += float64(c.R)
			colorSumG += float64(c.G)
			colorSumB += float64(c.B)
			colorCount++

			gray := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)

			// X-Gradient
			if prevGray >= 0 {
				if math.Abs(gray-prevGray) > 20 {
					edgeCount++
				}
			}

			// Y-Gradient (compare with pixel above if within rect)
			if y > rect.Min.Y {
				upC := color.NRGBAModel.Convert(img.At(x, y-1)).(color.NRGBA)
				upGray := 0.299*float64(upC.R) + 0.587*float64(upC.G) + 0.114*float64(upC.B)
				if math.Abs(gray-upGray) > 20 {
					edgeCount++
				}
			}

			prevGray = gray
		}
	}

	avgR := colorSumR / float64(colorCount)
	avgG := colorSumG / float64(colorCount)
	avgB := colorSumB / float64(colorCount)

	var colorVariance float64
	for y := rect.Min.Y; y < rect.Max.Y && y < bounds.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X && x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			colorVariance += math.Pow(float64(c.R)-avgR, 2) +
				math.Pow(float64(c.G)-avgG, 2) +
				math.Pow(float64(c.B)-avgB, 2)
		}
	}
	colorVariance /= float64(colorCount)

	blockArea := float64(rect.Dx() * rect.Dy())
	pageArea := float64(pageBounds.Dx() * pageBounds.Dy())

	return BlockMetrics{
		EdgeDensity:   float64(edgeCount) / blockArea,
		ColorVariance: colorVariance,
		AspectRatio:   float64(rect.Dx()) / float64(rect.Dy()),
		RelativeSize:  blockArea / pageArea,
	}
}

func (d *EnhancedDetector) classifyBlock(m BlockMetrics, rect image.Rectangle, pageBounds image.Rectangle) BlockType {
	relY := float64(rect.Min.Y) / float64(pageBounds.Dy())

	// Headers are usually at the top (top 15%)
	if relY < 0.15 && m.AspectRatio > 3.0 && m.EdgeDensity < 0.2 {
		return BlockTypeHeader
	}

	// Footers are usually at the bottom (bottom 10%)
	if relY > 0.90 && m.AspectRatio > 3.0 {
		return BlockTypeFooter
	}

	if m.EdgeDensity > 0.2 {
		if m.ColorVariance > 15000 {
			return BlockTypeDiagram
		}
		return BlockTypeText
	}
	if m.ColorVariance > 10000 {
		return BlockTypeImage
	}
	if m.EdgeDensity < 0.1 && m.ColorVariance < 1000 {
		return BlockTypeBackground
	}
	return BlockTypeUnknown
}
