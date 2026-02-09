package effects

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/ivlev/pdf2video/internal/config"
)

type Effect interface {
	GenerateFilter(params config.SegmentParams) string
}

type DefaultEffect struct{}

func (e *DefaultEffect) GenerateFilter(p config.SegmentParams) string {
	mode := strings.ToLower(p.ZoomMode)
	if mode == "random" || mode == "out-random" {
		modes := []string{"center", "top-left", "top-right", "bottom-left", "bottom-right"}
		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(p.PageIndex*99)))
		mode = modes[r.Intn(len(modes))]
	}

	var zoomX, zoomY string
	switch mode {
	case "top-left":
		zoomX, zoomY = "0", "0"
	case "top-right":
		zoomX, zoomY = "iw-(iw/zoom)", "0"
	case "bottom-left":
		zoomX, zoomY = "0", "ih-(ih/zoom)"
	case "bottom-right":
		zoomX, zoomY = "iw-(iw/zoom)", "ih-(ih/zoom)"
	default: // center
		zoomX, zoomY = "iw/2-(iw/zoom/2)", "ih/2-(ih/zoom/2)"
	}

	fFPS := float64(p.FPS)
	fTotal := p.Duration * fFPS
	fFade := p.FadeDuration * fFPS
	fActive := fTotal - fFade
	if fActive <= 0 {
		fActive = fTotal
	}

	zSpeed := p.ZoomSpeed
	if zSpeed <= 0 {
		zSpeed = 0.001
	}

	onPeak := 0.5 / zSpeed
	if onPeak > fActive/2 {
		onPeak = fActive / 2
	}

	actualPeak := 1.0 + (zSpeed * onPeak)
	if actualPeak > 1.5 {
		actualPeak = 1.5
		onPeak = 0.5 / zSpeed
	}

	zFormula := fmt.Sprintf("if(lte(on,%f), 1.0+(%f*on), if(lte(on,%f), %f-(%f-1.0)*(on-%f)/(%f-%f), 1.0))",
		onPeak, zSpeed, fActive, actualPeak, actualPeak, onPeak, fActive, onPeak)

	aspectFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width*2, p.Height*2, p.Width*2, p.Height*2,
	)

	zoomFilter := fmt.Sprintf(
		"zoompan=z='%s':d=%d:s=%dx%d:x='%s':y='%s'",
		zFormula, int(fTotal), p.Width, p.Height, zoomX, zoomY,
	)

	return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, zoomFilter, p.Width, p.Height)
}
