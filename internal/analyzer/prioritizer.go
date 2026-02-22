package analyzer

import (
	"math"
	"sort"
)

// BlockPrioritizer ranks blocks for display and flight planning
type BlockPrioritizer struct {
	ContentWeight  float64
	ScoreWeight    float64
	PositionWeight float64
	SizeWeight     float64
}

func NewBlockPrioritizer() *BlockPrioritizer {
	return &BlockPrioritizer{
		ContentWeight:  0.40,
		ScoreWeight:    0.25,
		PositionWeight: 0.20,
		SizeWeight:     0.15,
	}
}

// Prioritize calculates priority for each block and sorts them
func (p *BlockPrioritizer) Prioritize(blocks []Block, pageWidth, pageHeight int) []Block {
	if len(blocks) == 0 {
		return blocks
	}

	result := make([]Block, len(blocks))
	copy(result, blocks)

	for i := range result {
		result[i].Priority = p.calculatePriority(result[i], pageWidth, pageHeight)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

func (p *BlockPrioritizer) calculatePriority(block Block, pageWidth, pageHeight int) float64 {
	// 1. Content Type Priority
	contentPriority := p.getContentTypeScore(block.Type)

	// 2. Score Priority (from Detector)
	scorePriority := block.Score

	// 3. Position Priority (Top is better for reading flow)
	centerY := float64(block.Rect.Min.Y+block.Rect.Max.Y) / 2
	positionPriority := 1.0 - (centerY / float64(pageHeight))

	// 4. Size Priority (Aim for 10-20% of page area as optimal)
	area := float64(block.Rect.Dx() * block.Rect.Dy())
	pageArea := float64(pageWidth * pageHeight)
	relSize := area / pageArea

	// Bell-like curve around 15%
	sizePriority := math.Exp(-math.Pow(relSize-0.15, 2) / 0.05)

	return contentPriority*p.ContentWeight +
		scorePriority*p.ScoreWeight +
		positionPriority*p.PositionWeight +
		sizePriority*p.SizeWeight
}

func (p *BlockPrioritizer) getContentTypeScore(t BlockType) float64 {
	switch t {
	case BlockTypeHeader:
		return 1.0
	case BlockTypeChart, BlockTypeDiagram:
		return 0.9
	case BlockTypeText:
		return 0.7
	case BlockTypeImage:
		return 0.6
	case BlockTypeFooter:
		return 0.3
	case BlockTypeBackground:
		return 0.1
	default:
		return 0.5
	}
}
