package director

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

// SlideTimings holds the calculated durations for all parts of a slide
type SlideTimings struct {
	Intro    float64
	Outro    float64
	Fade     float64
	Dwell    []float64 // Indexed same as blocks
	Total    float64
}

// Director generates camera path scenarios from detected blocks
type Director struct {
	ViewportWidth  int
	ViewportHeight int
	MinDwell       float64 // Minimum time per block (seconds)
	MaxDwell       float64 // Maximum time per block (seconds)
}

// NewDirector creates a new Director with default settings
func NewDirector(viewportWidth, viewportHeight int) *Director {
	return &Director{
		ViewportWidth:  viewportWidth,
		ViewportHeight: viewportHeight,
		MinDwell:       1.0,
		MaxDwell:       3.0,
	}
}

// GenerateScenario creates a scenario from detected blocks
func (d *Director) GenerateScenario(blocks []analyzer.Block, input string, totalDuration, fadeDuration, outroDuration float64) (*Scenario, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks detected")
	}

	// Sort blocks in reading order (top-to-bottom, left-to-right)
	sortedBlocks := d.sortBlocks(blocks)

	// Calculate durations per block and slide components using adaptive logic
	timings := d.calculateDwellTimes(totalDuration, fadeDuration, outroDuration, sortedBlocks)

	// Generate keyframes
	keyframes := d.generateKeyframes(sortedBlocks, timings)

	slide := Slide{
		ID:        1,
		Input:     input,
		Duration:  totalDuration,
		Keyframes: keyframes,
	}

	scenario := &Scenario{
		Version: "1.0",
		Slides:  []Slide{slide},
	}

	return scenario, nil
}

// sortBlocks sorts blocks in reading order (Western: top-to-bottom, left-to-right)
func (d *Director) sortBlocks(blocks []analyzer.Block) []analyzer.Block {
	sorted := make([]analyzer.Block, len(blocks))
	copy(sorted, blocks)

	sort.Slice(sorted, func(i, j int) bool {
		// Threshold for "same row" (20 pixels)
		threshold := 20

		yDiff := sorted[i].Rect.Min.Y - sorted[j].Rect.Min.Y
		if abs(yDiff) > threshold {
			return sorted[i].Rect.Min.Y < sorted[j].Rect.Min.Y
		}

		// Same row, sort by X
		return sorted[i].Rect.Min.X < sorted[j].Rect.Min.X
	})

	return sorted
}

// calculateDwellTimes determines adaptive stay duration for each block based on importance and content.
// It ensures that Intro + Outro + Fade + sum(Dwell) == totalDuration.
func (d *Director) calculateDwellTimes(totalDuration, fadeDuration, outroDuration float64, blocks []analyzer.Block) SlideTimings {
	res := SlideTimings{
		Fade:  fadeDuration,
		Total: totalDuration,
	}

	if len(blocks) == 0 {
		return res
	}

	// 1. Handle Reserved Time (Intro/Outro/Fade)
	// We MUST respect fadeDuration as it's used for xfade in FFmpeg.
	// If totalDuration is too small even for fade, we scale fade down (though engine should prevent this).
	actualFade := fadeDuration
	if actualFade > totalDuration {
		actualFade = totalDuration
	}
	res.Fade = actualFade

	// Remaining for intro + outro + blocks
	remaining := totalDuration - actualFade
	introNominal := 1.0
	outroNominal := outroDuration

	// If remaining is not enough for intro + outro, scale them down proportionally
	if remaining < (introNominal + outroNominal) {
		if remaining <= 0 {
			res.Intro = 0
			res.Outro = 0
		} else {
			scale := remaining / (introNominal + outroNominal)
			res.Intro = introNominal * scale
			res.Outro = outroNominal * scale
		}
	} else {
		res.Intro = introNominal
		res.Outro = outroNominal
	}

	availableDuration := totalDuration - (res.Intro + res.Outro + res.Fade)
	if availableDuration < 0.001 {
		availableDuration = 0.001 // Minimum available for division
	}

	// 2. Assign weights
	weights := make([]float64, len(blocks))
	totalWeight := 0.0

	for i, b := range blocks {
		weight := b.Priority
		if weight == 0 {
			weight = 0.5
		}

		multiplier := 1.0
		switch b.Type {
		case analyzer.BlockTypeHeader:
			multiplier = 0.8
		case analyzer.BlockTypeChart, analyzer.BlockTypeDiagram:
			multiplier = 1.5
		case analyzer.BlockTypeImage:
			multiplier = 1.1
		case analyzer.BlockTypeText:
			multiplier = 1.2
		}
		weight *= multiplier

		if b.Metrics.EdgeDensity > 0 {
			weight *= (1.0 + b.Metrics.EdgeDensity)
		}

		weights[i] = weight
		totalWeight += weight
	}

	// 3. Distribute time
	durations := make([]float64, len(blocks))
	if totalWeight == 0 {
		equal := availableDuration / float64(len(blocks))
		for i := range durations {
			durations[i] = equal
		}
	} else {
		for i := range weights {
			durations[i] = (weights[i] / totalWeight) * availableDuration
		}
	}

	// 4. Clamping and redistribution
	// We use d.MinDwell but we must ensure we don't exceed availableDuration
	// If sum(minDwell) > availableDuration, we scale minDwell down
	minTotal := d.MinDwell * float64(len(blocks))
	effectiveMin := d.MinDwell
	if minTotal > availableDuration && availableDuration > 0 {
		effectiveMin = availableDuration / float64(len(blocks))
	}

	for iteration := 0; iteration < 3; iteration++ {
		surplus := 0.0
		adjustableIndices := []int{}

		for i := range durations {
			if durations[i] < effectiveMin {
				surplus += durations[i] - effectiveMin
				durations[i] = effectiveMin
			} else if durations[i] > d.MaxDwell {
				surplus += durations[i] - d.MaxDwell
				durations[i] = d.MaxDwell
			} else {
				adjustableIndices = append(adjustableIndices, i)
			}
		}

		if math.Abs(surplus) < 0.001 || len(adjustableIndices) == 0 {
			break
		}

		fragment := surplus / float64(len(adjustableIndices))
		for _, idx := range adjustableIndices {
			durations[idx] += fragment
		}
	}

	res.Dwell = durations
	return res
}

// generateKeyframes creates keyframes for camera movement using adaptive dwell durations and precise slide timings
func (d *Director) generateKeyframes(blocks []analyzer.Block, t SlideTimings) []Keyframe {
	keyframes := []Keyframe{}

	// Start with full view
	keyframes = append(keyframes, Keyframe{
		Time:  0.0,
		Focus: "full_view",
		Rect: Rectangle{
			X: 0,
			Y: 0,
			W: d.ViewportWidth,
			H: d.ViewportHeight,
		},
		Zoom: 1.0,
	})

	currentTime := t.Intro

	// Generate keyframes for each block using its adaptive duration
	for i, block := range blocks {
		zoom := d.calculateZoom(block.Rect)
		localDwell := t.Dwell[i]

		keyframes = append(keyframes, Keyframe{
			Time:  currentTime,
			Focus: fmt.Sprintf("region_%d", i+1),
			Rect: Rectangle{
				X: block.Rect.Min.X,
				Y: block.Rect.Min.Y,
				W: block.Rect.Dx(),
				H: block.Rect.Dy(),
			},
			Zoom: zoom,
		})

		currentTime += localDwell
	}

	// End of blocks, finish exactly outroDuration before the fade starts
	outroZoomOutStartTime := t.Total - t.Fade - t.Outro
	if outroZoomOutStartTime < currentTime {
		outroZoomOutStartTime = currentTime
	}

	// Fix the current state before zoom-out starts to ensure exact duration
	if len(blocks) > 0 {
		lastBlock := blocks[len(blocks)-1]
		keyframes = append(keyframes, Keyframe{
			Time:  outroZoomOutStartTime,
			Focus: "outro_stable",
			Rect: Rectangle{
				X: lastBlock.Rect.Min.X,
				Y: lastBlock.Rect.Min.Y,
				W: lastBlock.Rect.Dx(),
				H: lastBlock.Rect.Dy(),
			},
			Zoom: d.calculateZoom(lastBlock.Rect),
		})
	}

	// End with full view exactly when the transition starts
	keyframes = append(keyframes, Keyframe{
		Time:  t.Total - t.Fade,
		Focus: "full_view",
		Rect: Rectangle{
			X: 0,
			Y: 0,
			W: d.ViewportWidth,
			H: d.ViewportHeight,
		},
		Zoom: 1.0,
	})

	// Maintain full view during the crossfade
	keyframes = append(keyframes, Keyframe{
		Time:  t.Total,
		Focus: "full_view",
		Rect: Rectangle{
			X: 0,
			Y: 0,
			W: d.ViewportWidth,
			H: d.ViewportHeight,
		},
		Zoom: 1.0,
	})

	return keyframes
}

// calculateZoom determines zoom level to fit block in viewport
func (d *Director) calculateZoom(block image.Rectangle) float64 {
	padding := 0.9 // Use 90% of viewport

	viewportW := float64(d.ViewportWidth) * padding
	viewportH := float64(d.ViewportHeight) * padding

	blockW := float64(block.Dx())
	blockH := float64(block.Dy())

	if blockW == 0 || blockH == 0 {
		return 1.0
	}

	scaleX := viewportW / blockW
	scaleY := viewportH / blockH

	// Use the smaller scale to ensure block fits
	zoom := math.Min(scaleX, scaleY)

	// Clamp zoom to reasonable range
	if zoom < 1.0 {
		zoom = 1.0
	}
	if zoom > 3.0 {
		zoom = 3.0
	}

	return zoom
}

// calculateCenter finds the center point of a rectangle
func (d *Director) calculateCenter(rect image.Rectangle) image.Point {
	return image.Point{
		X: rect.Min.X + rect.Dx()/2,
		Y: rect.Min.Y + rect.Dy()/2,
	}
}

// abs returns absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
