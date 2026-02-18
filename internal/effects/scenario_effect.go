package effects

import (
	"fmt"
	"sort"

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

	// ДОРАБОТКА: Гарантированный возврат к 1:1 за OutroDuration до начала перехода
	// fadeStart - момент когда начинается xfade
	fadeStart := p.Duration - p.FadeDuration
	zoomOutStart := fadeStart - p.OutroDuration

	if zoomOutStart > 0 {
		// 1. Находим текущий зум в момент начала зум-аута (интерполяция по существующим кадрам)
		// Для простоты берем последний кадр перед zoomOutStart
		lastZoom := 1.0
		lastRect := director.Rectangle{X: 0, Y: 0, W: p.Width, H: p.Height}
		for _, kf := range scaledKeyframes {
			if kf.Time <= zoomOutStart {
				lastZoom = kf.Zoom
				lastRect = kf.Rect
			} else {
				break
			}
		}

		// 2. Инъекция кадра начала зум-аута (чтобы зафиксировать текущее положение)
		scaledKeyframes = append(scaledKeyframes, director.Keyframe{
			Time:  zoomOutStart,
			Focus: "zoom_out_start",
			Zoom:  lastZoom,
			Rect:  lastRect,
		})

		// 3. Инъекция кадра завершения зум-аута (1:1 за 1.5с)
		scaledKeyframes = append(scaledKeyframes, director.Keyframe{
			Time:  fadeStart,
			Focus: "full_view",
			Zoom:  1.0,
			Rect:  director.Rectangle{X: 0, Y: 0, W: p.Width, H: p.Height},
		})

		// 4. Удержание 1:1 во время перехода
		scaledKeyframes = append(scaledKeyframes, director.Keyframe{
			Time:  p.Duration,
			Focus: "full_view",
			Zoom:  1.0,
			Rect:  director.Rectangle{X: 0, Y: 0, W: p.Width, H: p.Height},
		})

		// 5. Обеспечиваем сортировку по времени
		sort.Slice(scaledKeyframes, func(i, j int) bool {
			return scaledKeyframes[i].Time < scaledKeyframes[j].Time
		})
	}

	// Используем генератор фильтра с масштабированной длительностью и кадрами
	zoomFilter := renderer.GenerateZoomPanFilter(scaledKeyframes, p.Duration, p.FPS, p.Width, p.Height)
	debugFilter := ""
	if p.Debug {
		boxFilter := renderer.GenerateDebugBoxFilter(scaledKeyframes, p.FPS, p.Width*2, p.Height*2)
		textFilter := fmt.Sprintf("drawtext=text='Slide %d | Time %%{pts\\:hms} | Zoom %%{zoom}':x=10:y=10:fontsize=24:fontcolor=yellow:box=1:boxcolor=black@0.5", p.PageIndex+1)
		debugFilter = fmt.Sprintf("%s,%s", boxFilter, textFilter)
	}

	// Aspect ratio handling (2x scale for better zoom quality as done in DefaultEffect)
	aspectFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width*2, p.Height*2, p.Width*2, p.Height*2,
	)

	if zoomFilter == "" {
		if debugFilter != "" {
			return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, debugFilter, p.Width, p.Height)
		}
		return fmt.Sprintf("%s,scale=%d:%d", aspectFilter, p.Width, p.Height)
	}

	if debugFilter != "" {
		// Draw box BEFORE zoompan (so it shows the target area)
		// Draw text AFTER zoompan (so it's readable)
		boxFilter := renderer.GenerateDebugBoxFilter(scaledKeyframes, p.FPS, p.Width*2, p.Height*2)
		textFilter := fmt.Sprintf("drawtext=text='Slide %d | Zoom %%{zoom}':x=10:y=10:fontsize=24:fontcolor=yellow:box=1:boxcolor=black@0.5", p.PageIndex+1)
		return fmt.Sprintf("%s,%s,%s,%s,scale=%d:%d", aspectFilter, boxFilter, zoomFilter, textFilter, p.Width, p.Height)
	}

	return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, zoomFilter, p.Width, p.Height)
}
