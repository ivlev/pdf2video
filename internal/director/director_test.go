package director

import (
	"image"
	"testing"

	"github.com/ivlev/pdf2video/internal/analyzer"
)

func TestDirector(t *testing.T) {
	director := NewDirector(1280, 720)

	// Create test blocks
	blocks := []analyzer.Block{
		{
			Rect:       image.Rect(50, 50, 200, 100),
			Type:       "text",
			Confidence: 0.8,
		},
		{
			Rect:       image.Rect(50, 150, 300, 250),
			Type:       "text",
			Confidence: 0.9,
		},
	}

	scenario, err := director.GenerateScenario(blocks, "test.png", 10.0, 0.5, 1.0)
	if err != nil {
		t.Fatalf("GenerateScenario failed: %v", err)
	}

	if scenario.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", scenario.Version)
	}

	if len(scenario.Slides) != 1 {
		t.Fatalf("Expected 1 slide, got %d", len(scenario.Slides))
	}

	slide := scenario.Slides[0]
	if slide.Input != "test.png" {
		t.Errorf("Expected input test.png, got %s", slide.Input)
	}

	if slide.Duration != 10.0 {
		t.Errorf("Expected duration 10.0, got %f", slide.Duration)
	}

	// Should have: intro + 2 blocks + outro = 4 keyframes
	if len(slide.Keyframes) < 3 {
		t.Errorf("Expected at least 3 keyframes, got %d", len(slide.Keyframes))
	}

	t.Logf("Generated scenario with %d keyframes", len(slide.Keyframes))
	for i, kf := range slide.Keyframes {
		t.Logf("Keyframe %d: time=%.1fs, focus=%s, zoom=%.2f", i, kf.Time, kf.Focus, kf.Zoom)
	}
}

func TestScenarioWriteRead(t *testing.T) {
	scenario := &Scenario{
		Version: "1.0",
		Slides: []Slide{
			{
				ID:       1,
				Input:    "test.png",
				Duration: 5.0,
				Keyframes: []Keyframe{
					{Time: 0.0, Focus: "full", Rect: Rectangle{X: 0, Y: 0, W: 1280, H: 720}, Zoom: 1.0},
					{Time: 2.5, Focus: "block1", Rect: Rectangle{X: 100, Y: 100, W: 200, H: 150}, Zoom: 1.5},
				},
			},
		},
	}

	// Write
	tmpFile := "/tmp/test_scenario.yaml"
	if err := WriteScenario(scenario, tmpFile); err != nil {
		t.Fatalf("WriteScenario failed: %v", err)
	}

	// Read
	readScenario, err := ReadScenario(tmpFile)
	if err != nil {
		t.Fatalf("ReadScenario failed: %v", err)
	}

	if readScenario.Version != scenario.Version {
		t.Errorf("Version mismatch: expected %s, got %s", scenario.Version, readScenario.Version)
	}

	if len(readScenario.Slides) != len(scenario.Slides) {
		t.Errorf("Slide count mismatch: expected %d, got %d", len(scenario.Slides), len(readScenario.Slides))
	}
}
