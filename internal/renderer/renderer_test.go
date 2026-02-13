package renderer

import (
	"testing"

	"github.com/ivlev/pdf2video/internal/director"
)

func TestInterpolateKeyframes(t *testing.T) {
	keyframes := []director.Keyframe{
		{Time: 0.0, Rect: director.Rectangle{X: 0, Y: 0, W: 1920, H: 1080}, Zoom: 1.0},
		{Time: 2.0, Rect: director.Rectangle{X: 100, Y: 100, W: 800, H: 600}, Zoom: 1.5},
		{Time: 4.0, Rect: director.Rectangle{X: 200, Y: 200, W: 400, H: 300}, Zoom: 2.0},
	}

	tests := []struct {
		time         float64
		expectedZoom float64
	}{
		{0.0, 1.0},  // First keyframe
		{1.0, 1.25}, // Midpoint between first and second (approximately)
		{2.0, 1.5},  // Second keyframe
		{3.0, 1.75}, // Midpoint between second and third (approximately)
		{4.0, 2.0},  // Third keyframe
		{5.0, 2.0},  // After last keyframe
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			state := InterpolateKeyframes(keyframes, tt.time)

			// Allow some tolerance due to easing
			tolerance := 0.3
			if abs(state.Zoom-tt.expectedZoom) > tolerance {
				t.Errorf("At time %.1f: expected zoom ~%.2f, got %.2f", tt.time, tt.expectedZoom, state.Zoom)
			}
		})
	}
}

func TestGenerateZoomPanFilter(t *testing.T) {
	keyframes := []director.Keyframe{
		{Time: 0.0, Rect: director.Rectangle{X: 0, Y: 0, W: 1920, H: 1080}, Zoom: 1.0},
		{Time: 2.0, Rect: director.Rectangle{X: 100, Y: 100, W: 800, H: 600}, Zoom: 1.5},
	}

	filter := GenerateZoomPanFilter(keyframes, 3.0, 30, 1920, 1080)

	if filter == "" {
		t.Error("Expected non-empty filter")
	}

	// Check that filter contains expected components
	if !contains(filter, "zoompan") {
		t.Error("Filter should contain 'zoompan'")
	}
	if !contains(filter, "z='") {
		t.Error("Filter should contain zoom expression")
	}
	if !contains(filter, "x='") {
		t.Error("Filter should contain x expression")
	}
	if !contains(filter, "y='") {
		t.Error("Filter should contain y expression")
	}

	t.Logf("Generated filter: %s", filter)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
