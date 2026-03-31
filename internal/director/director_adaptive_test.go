package director

import (
	"image"
	"math"
	"testing"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

func TestCalculateDwellTimes(t *testing.T) {
	d := NewDirector(1280, 720)
	d.MinDwell = 1.0
	d.MaxDwell = 5.0

	blocks := []analyzer.Block{
		{
			Type:     analyzer.BlockTypeHeader,
			Priority: 0.8,
			Rect:     image.Rect(0, 0, 100, 100),
		},
		{
			Type:     analyzer.BlockTypeChart,
			Priority: 0.9,
			Metrics: analyzer.BlockMetrics{
				EdgeDensity: 0.5,
			},
			Rect: image.Rect(0, 0, 100, 100),
		},
		{
			Type:     analyzer.BlockTypeText,
			Priority: 0.7,
			Rect:     image.Rect(0, 0, 100, 100),
		},
	}

	totalDuration := 15.0
	fadeDuration := 0.5
	outroDuration := 1.0

	durations := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, blocks)

	if len(durations) != len(blocks) {
		t.Fatalf("expected %d durations, got %d", len(blocks), len(durations))
	}

	// Verify total duration
	sum := 0.0
	for _, v := range durations {
		sum += v
	}

	introDuration := 1.0
	expectedAvailable := totalDuration - fadeDuration - outroDuration - introDuration
	if math.Abs(sum-expectedAvailable) > 0.001 {
		t.Errorf("expected total dwell time %f, got %f", expectedAvailable, sum)
	}

	// Verify that Chart (index 1) has more time than Header (index 0)
	if durations[1] <= durations[0] {
		t.Errorf("expected Chart to have more time than Header, got %f vs %f", durations[1], durations[0])
	}
}

func TestCalculateDwellTimes_Clamping(t *testing.T) {
	d := NewDirector(1280, 720)
	d.MinDwell = 2.0
	d.MaxDwell = 3.0

	blocks := []analyzer.Block{
		{Priority: 0.1}, // Small weight
		{Priority: 0.9}, // Large weight
	}

	totalDuration := 10.0 // intro(1) + outro(1) + fade(1) = 3s reserved. Available = 7s.
	fadeDuration := 1.0
	outroDuration := 1.0

	durations := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, blocks)

	for i, v := range durations {
		if v < d.MinDwell {
			t.Errorf("block %d duration %f below MinDwell %f", i, v, d.MinDwell)
		}
		if v > d.MaxDwell {
			t.Errorf("block %d duration %f above MaxDwell %f", i, v, d.MaxDwell)
		}
	}
}
