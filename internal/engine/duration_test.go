package engine

import (
	"math"
	"testing"

	"github.com/ivlev/pdf2video/internal/config"
)

func TestCalculateDurations(t *testing.T) {
	cfg := &config.Config{
		TotalDuration: 100.0, // A
		FadeDuration:  0.5,   // F
	}
	project := &VideoProject{Config: cfg}

	pageCount := 10 // N
	project.calculateDurations(pageCount)

	durations := cfg.PageDurations
	if len(durations) != pageCount {
		t.Errorf("Expected %d durations, got %d", pageCount, len(durations))
	}

	// 1. Check total duration
	// sum(D_i) - (N-1)*F should equal A
	sum := 0.0
	for _, d := range durations {
		sum += d
	}

	expectedSum := cfg.TotalDuration + float64(pageCount-1)*cfg.FadeDuration
	if math.Abs(sum-expectedSum) > 0.0001 {
		t.Errorf("Expected sum %f, got %f (diff %f)", expectedSum, sum, math.Abs(sum-expectedSum))
	}

	// 2. Check first clip variation
	Dbase := expectedSum / float64(pageCount)
	variation0 := (durations[0] / Dbase) - 1.0
	if math.Abs(variation0) > 0.1501 {
		t.Errorf("First clip variation too high: %f", variation0)
	}

	// 3. Check subsequent clip variation
	for i := 1; i < pageCount; i++ {
		variation := (durations[i] / durations[i-1]) - 1.0
		if math.Abs(variation) > 0.1501 {
			t.Errorf("Clip %d variation too high: %f (prev: %f, curr: %f)", i, variation, durations[i-1], durations[i])
		}
	}
}
