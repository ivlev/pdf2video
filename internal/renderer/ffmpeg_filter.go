package renderer

import (
	"fmt"

	"github.com/ivlev/pdf2video/internal/director"
)

// GenerateZoomPanFilter creates FFmpeg zoompan filter for scenario-based rendering
func GenerateZoomPanFilter(keyframes []director.Keyframe, duration float64, fps int, width, height int) string {
	if len(keyframes) == 0 {
		return ""
	}

	// Build zoompan filter with piecewise expressions
	zoomExpr := buildZoomExpression(keyframes, fps)
	xExpr := buildPanExpression(keyframes, fps, width, true)
	yExpr := buildPanExpression(keyframes, fps, height, false)

	return fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=1:s=%dx%d:fps=%d",
		zoomExpr, xExpr, yExpr, width, height, fps)
}

// buildZoomExpression creates piecewise zoom expression for FFmpeg
func buildZoomExpression(keyframes []director.Keyframe, fps int) string {
	if len(keyframes) == 1 {
		return fmt.Sprintf("%.6f", keyframes[0].Zoom)
	}

	expr := ""
	for i := 0; i < len(keyframes)-1; i++ {
		startFrame := int(keyframes[i].Time * float64(fps))
		endFrame := int(keyframes[i+1].Time * float64(fps))
		startZoom := keyframes[i].Zoom
		endZoom := keyframes[i+1].Zoom

		if i > 0 {
			expr += ","
		}

		// Linear interpolation between keyframes
		// if(lte(on,endFrame),startZoom+(on-startFrame)/(endFrame-startFrame)*(endZoom-startZoom),...)
		if endFrame > startFrame {
			expr += fmt.Sprintf("if(lte(on,%d),%.6f+(on-%d)/(%d-%.6f)*(%.6f-%.6f)",
				endFrame, startZoom, startFrame, endFrame-startFrame, startZoom, endZoom, startZoom)
		} else {
			expr += fmt.Sprintf("%.6f", startZoom)
		}
	}

	// Close all if statements and add final zoom
	for i := 0; i < len(keyframes)-2; i++ {
		expr += ")"
	}
	expr += fmt.Sprintf(",%.6f)", keyframes[len(keyframes)-1].Zoom)

	return expr
}

// buildPanExpression creates piecewise pan expression for X or Y axis
func buildPanExpression(keyframes []director.Keyframe, fps int, dimension int, isX bool) string {
	if len(keyframes) == 1 {
		center := getCenter(keyframes[0], isX)
		return fmt.Sprintf("%.6f", float64(dimension)/2.0-center)
	}

	expr := ""
	for i := 0; i < len(keyframes)-1; i++ {
		startFrame := int(keyframes[i].Time * float64(fps))
		endFrame := int(keyframes[i+1].Time * float64(fps))
		startCenter := getCenter(keyframes[i], isX)
		endCenter := getCenter(keyframes[i+1], isX)

		if i > 0 {
			expr += ","
		}

		// Pan to keep region centered
		// x = viewport_width/2 - region_center_x
		if endFrame > startFrame {
			expr += fmt.Sprintf("if(lte(on,%d),%.6f+(on-%d)/(%d)*(%.6f-%.6f)",
				endFrame,
				float64(dimension)/2.0-startCenter,
				startFrame,
				endFrame-startFrame,
				float64(dimension)/2.0-endCenter,
				float64(dimension)/2.0-startCenter)
		} else {
			expr += fmt.Sprintf("%.6f", float64(dimension)/2.0-startCenter)
		}
	}

	// Close all if statements and add final position
	for i := 0; i < len(keyframes)-2; i++ {
		expr += ")"
	}
	finalCenter := getCenter(keyframes[len(keyframes)-1], isX)
	expr += fmt.Sprintf(",%.6f)", float64(dimension)/2.0-finalCenter)

	return expr
}

// getCenter extracts center coordinate from keyframe rectangle
func getCenter(kf director.Keyframe, isX bool) float64 {
	if isX {
		return float64(kf.Rect.X + kf.Rect.W/2)
	}
	return float64(kf.Rect.Y + kf.Rect.H/2)
}
