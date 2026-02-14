package director

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

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

	// Calculate duration per block
	dwellTime := d.calculateDwellTime(totalDuration, fadeDuration, outroDuration, len(sortedBlocks))

	// Generate keyframes
	keyframes := d.generateKeyframes(sortedBlocks, dwellTime, totalDuration, fadeDuration, outroDuration)

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

// calculateDwellTime determines how long to show each block
func (d *Director) calculateDwellTime(totalDuration, fadeDuration, outroDuration float64, blockCount int) float64 {
	// Reserve time for intro (1s) and outro zoom-out (outroDuration)
	// Outro zoom-out must finish before the fade starts
	introDuration := 1.0
	reservedDuration := introDuration + outroDuration + fadeDuration
	availableDuration := totalDuration - reservedDuration

	if availableDuration <= 0 {
		availableDuration = totalDuration
	}

	dwellTime := availableDuration / float64(blockCount)

	// Clamp to min/max
	if dwellTime < d.MinDwell {
		dwellTime = d.MinDwell
	}
	if dwellTime > d.MaxDwell {
		dwellTime = d.MaxDwell
	}

	return dwellTime
}

// generateKeyframes creates keyframes for camera movement
func (d *Director) generateKeyframes(blocks []analyzer.Block, dwellTime, totalDuration, fadeDuration, outroDuration float64) []Keyframe {
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

	currentTime := 1.0 // 1s intro

	// Generate keyframes for each block
	for i, block := range blocks {
		zoom := d.calculateZoom(block.Rect)

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

		currentTime += dwellTime
	}

	// End of blocks, finish exactly outroDuration before the fade starts
	// Unless we are already past that time (dwellTime too short)
	outroZoomOutStartTime := totalDuration - fadeDuration - outroDuration
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
		Time:  totalDuration - fadeDuration,
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
		Time:  totalDuration,
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
