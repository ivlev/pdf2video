package effects

import (
	"fmt"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/director"
	"github.com/ivlev/pdf2video/internal/renderer"
)

// ScenarioEffect uses a YAML scenario for camera movement
type ScenarioEffect struct {
	Scenario *director.Scenario
}

// NewScenarioEffect creates a new ScenarioEffect
func NewScenarioEffect(scenario *director.Scenario) *ScenarioEffect {
	return &ScenarioEffect{
		Scenario: scenario,
	}
}

// GenerateFilter generates FFmpeg filter for a specific slide from the scenario
func (e *ScenarioEffect) GenerateFilter(p config.SegmentParams) string {
	if e.Scenario == nil || p.PageIndex >= len(e.Scenario.Slides) {
		// Fallback to default behavior or static view if slide not found
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,scale=%d:%d",
			p.Width, p.Height, p.Width, p.Height, p.Width, p.Height)
	}

	slide := e.Scenario.Slides[p.PageIndex]

	// Масштабируем ключевые кадры под реальную длительность (рассчитанную движком)
	scaledKeyframes := make([]director.Keyframe, len(slide.Keyframes))
	timeScale := 1.0
	if slide.Duration > 0 {
		timeScale = p.Duration / slide.Duration
	}

	for i, kf := range slide.Keyframes {
		scaledKeyframes[i] = kf
		scaledKeyframes[i].Time *= timeScale
	}

	// Используем генератор фильтра с масштабированной длительностью и кадрами
	zoomFilter := renderer.GenerateZoomPanFilter(scaledKeyframes, p.Duration, p.FPS, p.Width, p.Height)

	// Aspect ratio handling (2x scale for better zoom quality as done in DefaultEffect)
	aspectFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width*2, p.Height*2, p.Width*2, p.Height*2,
	)

	if zoomFilter == "" {
		return fmt.Sprintf("%s,scale=%d:%d", aspectFilter, p.Width, p.Height)
	}

	return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, zoomFilter, p.Width, p.Height)
}
