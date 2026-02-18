package renderer

import (
	"fmt"
	"strings"

	"github.com/ivlev/pdf2video/internal/director"
)

// GenerateZoomPanFilter creates FFmpeg zoompan filter for scenario-based rendering
func GenerateZoomPanFilter(keyframes []director.Keyframe, duration float64, fps int, width, height int) string {
	if len(keyframes) == 0 {
		return ""
	}

	totalFrames := int(duration * float64(fps))
	if totalFrames <= 0 {
		totalFrames = 1
	}

	// Build zoompan filter with piecewise expressions
	zoomExpr := buildZoomExpression(keyframes, fps)
	xExpr := buildPanExpression(keyframes, fps, width, true)
	yExpr := buildPanExpression(keyframes, fps, height, false)

	return fmt.Sprintf("zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d",
		zoomExpr, xExpr, yExpr, totalFrames, width, height, fps)
}

// GenerateDebugBoxFilter creates a drawbox filter that matches the zoompan focus
func GenerateDebugBoxFilter(keyframes []director.Keyframe, fps int, width, height int) string {
	if len(keyframes) == 0 {
		return ""
	}
	zoomExpr := buildZoomExpression(keyframes, fps)
	// Actually, zoompan's x/y are the top-left of the CROP.
	// So drawbox at (x, y) with (iw/zoom, ih/zoom) should be exactly the crop area.
	xPan := buildPanExpression(keyframes, fps, width, true)
	yPan := buildPanExpression(keyframes, fps, height, false)

	return fmt.Sprintf("drawbox=x='%s':y='%s':w='iw/(%s)':h='ih/(%s)':color=red:t=5",
		xPan, yPan, zoomExpr, zoomExpr)
}

// buildZoomExpression creates piecewise zoom expression for FFmpeg
func buildZoomExpression(keyframes []director.Keyframe, fps int) string {
	if len(keyframes) == 0 {
		return "1"
	}
	if len(keyframes) == 1 {
		return fmt.Sprintf("%.6f", keyframes[0].Zoom)
	}

	exprParts := []string{}
	for i := 0; i < len(keyframes)-1; i++ {
		startFrame := int(keyframes[i].Time * float64(fps))
		endFrame := int(keyframes[i+1].Time * float64(fps))
		startZoom := keyframes[i].Zoom
		endZoom := keyframes[i+1].Zoom

		if endFrame > startFrame {
			// val = startZoom + (on - startFrame) * (endZoom - startZoom) / (endFrame - startFrame)
			part := fmt.Sprintf("between(on,%d,%d)*(%.6f+(on-%d)*(%.6f-%.6f)/(%d))",
				startFrame, endFrame-1, startZoom, startFrame, endZoom, startZoom, endFrame-startFrame)
			exprParts = append(exprParts, part)
		}
	}

	// Add final value for frame >= last keyframe
	lastFrame := int(keyframes[len(keyframes)-1].Time * float64(fps))
	lastZoom := keyframes[len(keyframes)-1].Zoom
	exprParts = append(exprParts, fmt.Sprintf("gte(on,%d)*%.6f", lastFrame, lastZoom))

	return strings.Join(exprParts, "+")
}

// buildPanExpression creates piecewise pan expression for X or Y axis
func buildPanExpression(keyframes []director.Keyframe, fps int, dimension int, isX bool) string {
	if len(keyframes) == 0 {
		return "0"
	}
	if len(keyframes) == 1 {
		center := getCenter(keyframes[0], isX)
		// x = center - (dim/zoom)/2
		return fmt.Sprintf("%.6f-(%d/zoom)/2", center, dimension)
	}

	exprParts := []string{}
	for i := 0; i < len(keyframes)-1; i++ {
		startFrame := int(keyframes[i].Time * float64(fps))
		endFrame := int(keyframes[i+1].Time * float64(fps))
		startCenter := getCenter(keyframes[i], isX)
		endCenter := getCenter(keyframes[i+1], isX)

		if endFrame > startFrame {
			// center = startCenter + (on - startFrame) * (endCenter - startCenter) / (endFrame - startFrame)
			part := fmt.Sprintf("between(on,%d,%d)*(%.6f+(on-%d)*(%.6f-%.6f)/(%d)-(%d/zoom)/2)",
				startFrame, endFrame-1, startCenter, startFrame, endCenter, startCenter, endFrame-startFrame, dimension)
			exprParts = append(exprParts, part)
		}
	}

	// Add final value for frame >= last keyframe
	lastFrame := int(keyframes[len(keyframes)-1].Time * float64(fps))
	lastCenter := getCenter(keyframes[len(keyframes)-1], isX)
	exprParts = append(exprParts, fmt.Sprintf("gte(on,%d)*(%.6f-(%d/zoom)/2)", lastFrame, lastCenter, dimension))

	return strings.Join(exprParts, "+")
}

// getCenter extracts center coordinate from keyframe rectangle
func getCenter(kf director.Keyframe, isX bool) float64 {
	if isX {
		return float64(kf.Rect.X + kf.Rect.W/2)
	}
	return float64(kf.Rect.Y + kf.Rect.H/2)
}
