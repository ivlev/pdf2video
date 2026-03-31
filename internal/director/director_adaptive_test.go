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

	timings := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, blocks)

	if len(timings.Dwell) != len(blocks) {
		t.Fatalf("expected %d durations, got %d", len(blocks), len(timings.Dwell))
	}

	// Verify total duration
	sum := timings.Intro + timings.Outro + timings.Fade
	for _, v := range timings.Dwell {
		sum += v
	}

	if math.Abs(sum-totalDuration) > 0.001 {
		t.Errorf("expected total dwell time %f, got %f", totalDuration, sum)
	}

	// Verify that Chart (index 1) has more time than Header (index 0)
	if timings.Dwell[1] <= timings.Dwell[0] {
		t.Errorf("expected Chart to have more time than Header, got %f vs %f", timings.Dwell[1], timings.Dwell[0])
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

	totalDuration := 10.0 // Available = 10 - (1 + 1 + 1) = 7s.
	fadeDuration := 1.0
	outroDuration := 1.0

	timings := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, blocks)

	for i, v := range timings.Dwell {
		if v < 2.0 { // d.MinDwell is not strictly attainable if sum(min) > available, but here 2*2=4 < 7.
			t.Errorf("block %d duration %f below expected min", i, v)
		}
		if v > d.MaxDwell {
			t.Errorf("block %d duration %f above MaxDwell %f", i, v, d.MaxDwell)
		}
	}
}

func TestCalculateDwellTimes_ShortSlideSync(t *testing.T) {
	d := NewDirector(1280, 720)
	// Normal reserved: intro(1.0) + outro(1.0) + fade(0.5) = 2.5s
	// Total: 2.0s
	totalDuration := 2.0
	fadeDuration := 0.5
	outroDuration := 1.0

	blocks := []analyzer.Block{
		{Priority: 0.5},
	}

	timings := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, blocks)

	// Check if total matches
	sum := timings.Intro + timings.Outro + timings.Fade
	for _, dv := range timings.Dwell {
		sum += dv
	}

	if math.Abs(sum-totalDuration) > 0.001 {
		t.Errorf("expected total %f, got %f", totalDuration, sum)
	}

	// Intro and Outro should be scaled down
	// remaining = 2.0 - 0.5 = 1.5
	// nominal intro(1.0) + outro(1.0) = 2.0
	// scale = 1.5 / 2.0 = 0.75
	// expected intro = 0.75, outro = 0.75
	if math.Abs(timings.Intro-0.75) > 0.01 {
		t.Errorf("expected intro 0.75, got %f", timings.Intro)
	}
}
