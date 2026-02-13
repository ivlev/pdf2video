package renderer

import (
	"github.com/ivlev/pdf2video/internal/director"
)

// CameraState represents the camera position and zoom at a specific moment
type CameraState struct {
	X    float64 // Pan X position (center point in pixels)
	Y    float64 // Pan Y position (center point in pixels)
	Zoom float64 // Zoom level (1.0 = no zoom)
}

// InterpolateKeyframes calculates camera state at a given time by interpolating between keyframes
func InterpolateKeyframes(keyframes []director.Keyframe, currentTime float64) CameraState {
	if len(keyframes) == 0 {
		return CameraState{X: 0, Y: 0, Zoom: 1.0}
	}

	// If before first keyframe, use first keyframe
	if currentTime <= keyframes[0].Time {
		kf := keyframes[0]
		return CameraState{
			X:    float64(kf.Rect.X + kf.Rect.W/2),
			Y:    float64(kf.Rect.Y + kf.Rect.H/2),
			Zoom: kf.Zoom,
		}
	}

	// If after last keyframe, use last keyframe
	if currentTime >= keyframes[len(keyframes)-1].Time {
		kf := keyframes[len(keyframes)-1]
		return CameraState{
			X:    float64(kf.Rect.X + kf.Rect.W/2),
			Y:    float64(kf.Rect.Y + kf.Rect.H/2),
			Zoom: kf.Zoom,
		}
	}

	// Find surrounding keyframes
	var prevKf, nextKf director.Keyframe
	for i := 0; i < len(keyframes)-1; i++ {
		if currentTime >= keyframes[i].Time && currentTime < keyframes[i+1].Time {
			prevKf = keyframes[i]
			nextKf = keyframes[i+1]
			break
		}
	}

	// Calculate interpolation factor (0.0 to 1.0)
	timeDelta := nextKf.Time - prevKf.Time
	if timeDelta == 0 {
		timeDelta = 0.001 // Avoid division by zero
	}
	t := (currentTime - prevKf.Time) / timeDelta

	// Apply easing (smooth in-out)
	t = easeInOutCubic(t)

	// Interpolate positions
	prevX := float64(prevKf.Rect.X + prevKf.Rect.W/2)
	prevY := float64(prevKf.Rect.Y + prevKf.Rect.H/2)
	nextX := float64(nextKf.Rect.X + nextKf.Rect.W/2)
	nextY := float64(nextKf.Rect.Y + nextKf.Rect.H/2)

	return CameraState{
		X:    lerp(prevX, nextX, t),
		Y:    lerp(prevY, nextY, t),
		Zoom: lerp(prevKf.Zoom, nextKf.Zoom, t),
	}
}

// lerp performs linear interpolation between a and b
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

// easeInOutCubic applies smooth easing function
func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - pow(-2*t+2, 3)/2
}

// pow calculates x^n
func pow(x float64, n int) float64 {
	result := 1.0
	for i := 0; i < n; i++ {
		result *= x
	}
	return result
}
