package director

import (
	"image"
	"testing"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

func TestTrajectoryOptimizer_Optimize(t *testing.T) {
	optimizer := NewTrajectoryOptimizer()

	// Start point: center of 1000x1000 page
	startPoint := image.Point{X: 500, Y: 500}

	blocks := []analyzer.Block{
		{
			Rect:     image.Rect(100, 100, 200, 200), // Top left, Priority 0.8
			Priority: 0.8,
			Score:    0.8,
		},
		{
			Rect:     image.Rect(800, 800, 900, 900), // Bottom right, Priority 0.9 (Highest, but far)
			Priority: 0.9,
			Score:    0.9,
		},
		{
			Rect:     image.Rect(450, 450, 550, 550), // Center, Priority 0.5 (Lowest, but closest)
			Priority: 0.5,
			Score:    0.5,
		},
	}

	optimized := optimizer.Optimize(blocks, startPoint)

	if len(optimized) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(optimized))
	}

	// With PriorityWeight=0.6 and DistanceWeight=0.4:
	// Starting from (500, 500):
	// Block 2 (Center) distance is 0. Score = 0.5*0.6 + 1.0*0.4 = 0.7
	// Block 1 (Top Left) dist ~ 424.
	// Block 2 (Bottom Right) dist ~ 424.
	// Center should likely be first because distance is 0.

	if optimized[0].Rect.Min.X != 450 {
		t.Errorf("Expected first block to be at center, got %v", optimized[0].Rect)
	}

	t.Logf("Optimization order:")
	for i, b := range optimized {
		t.Logf("%d: %v (Priority: %.2f)", i, b.Rect, b.Priority)
	}
}

func TestTrajectoryOptimizer_Empty(t *testing.T) {
	optimizer := NewTrajectoryOptimizer()
	optimized := optimizer.Optimize([]analyzer.Block{}, image.Point{0, 0})
	if len(optimized) != 0 {
		t.Errorf("Expected empty result, got %d", len(optimized))
	}
}
