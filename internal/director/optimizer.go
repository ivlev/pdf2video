package director

import (
	"image"
	"math"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

// TrajectoryOptimizer selects the best path through detected blocks
type TrajectoryOptimizer struct {
	// PriorityWeight determines how much we favor high-priority blocks (0.0 - 1.0)
	PriorityWeight float64
	// DistanceWeight determines how much we penalize long jumps (0.0 - 1.0)
	DistanceWeight float64
}

func NewTrajectoryOptimizer() *TrajectoryOptimizer {
	return &TrajectoryOptimizer{
		PriorityWeight: 0.6,
		DistanceWeight: 0.4,
	}
}

// Optimize sorts blocks using a greedy algorithm that balances priority and proximity
func (o *TrajectoryOptimizer) Optimize(blocks []analyzer.Block, startPoint image.Point) []analyzer.Block {
	if len(blocks) <= 1 {
		return blocks
	}

	unvisited := make([]analyzer.Block, len(blocks))
	copy(unvisited, blocks)

	optimized := make([]analyzer.Block, 0, len(blocks))
	currentPos := startPoint

	for len(unvisited) > 0 {
		nextIdx := o.findNext(unvisited, currentPos)
		nextBlock := unvisited[nextIdx]

		optimized = append(optimized, nextBlock)
		currentPos = calculateCenter(nextBlock.Rect)

		// Remove from unvisited
		unvisited = append(unvisited[:nextIdx], unvisited[nextIdx+1:]...)
	}

	return optimized
}

func (o *TrajectoryOptimizer) findNext(blocks []analyzer.Block, from image.Point) int {
	bestIdx := 0
	maxScore := -1.0

	// Find max distance for normalization
	maxDist := 1.0
	for _, b := range blocks {
		dist := distance(from, calculateCenter(b.Rect))
		if dist > maxDist {
			maxDist = dist
		}
	}

	for i, b := range blocks {
		// Norm priority (0.0 - 1.0) - assume analyzer already provides normalized priority
		prio := b.Priority
		if prio == 0 {
			prio = b.Score // Fallback to score
		}

		// Norm distance (0.0 - 1.0, where 0 is far, 1 is close)
		dist := distance(from, calculateCenter(b.Rect))
		distScore := 1.0 - (dist / maxDist)

		totalScore := prio*o.PriorityWeight + distScore*o.DistanceWeight

		if totalScore > maxScore {
			maxScore = totalScore
			bestIdx = i
		}
	}

	return bestIdx
}

func distance(p1, p2 image.Point) float64 {
	dx := float64(p1.X - p2.X)
	dy := float64(p1.Y - p2.Y)
	return math.Sqrt(dx*dx + dy*dy)
}

func calculateCenter(rect image.Rectangle) image.Point {
	return image.Point{
		X: rect.Min.X + rect.Dx()/2,
		Y: rect.Min.Y + rect.Dy()/2,
	}
}
