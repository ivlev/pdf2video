package effects

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/ivlev/pdf2video/internal/config"
	"github.com/ivlev/pdf2video/internal/system"
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

	// Peak calculation based on speed and available active time
	onPeak := 0.5 / zSpeed
	if onPeak > (fActive-p.OutroDuration*fFPS)/2 && (fActive-p.OutroDuration*fFPS) > 0 {
		onPeak = (fActive - p.OutroDuration*fFPS) / 2
	}

	actualPeak := 1.0 + (zSpeed * onPeak)
	if actualPeak > 1.5 {
		actualPeak = 1.5
		onPeak = 0.5 / zSpeed
	}

	// outroStart - moment when we start zooming back to 1:1
	outroStart := fActive - p.OutroDuration*fFPS
	if outroStart < onPeak {
		outroStart = onPeak
	}

	zFormula := fmt.Sprintf("if(lte(on,%f), 1.0+(%f*on), if(lte(on,%f), %f, if(lte(on,%f), %f-(%f-1.0)*(on-%f)/(%f-%f), 1.0)))",
		onPeak, zSpeed, outroStart, actualPeak, fActive, actualPeak, actualPeak, outroStart, fActive, outroStart)

	aspectFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width*2, p.Height*2, p.Width*2, p.Height*2,
	)

	zoomFilter := fmt.Sprintf(
		"zoompan=z='%s':d=%d:s=%dx%d:x='%s':y='%s':fps=%d",
		zFormula, int(fTotal), p.Width, p.Height, zoomX, zoomY, p.FPS,
	)

	if p.Debug && system.CheckFilterSupport("drawtext") {
		textFilter := fmt.Sprintf("drawtext=text='Slide %d':x=10:y=10:fontsize=24:fontcolor=yellow:box=1:boxcolor=black@0.5", p.PageIndex+1)
		return fmt.Sprintf("%s,%s,%s,scale=%d:%d", aspectFilter, zoomFilter, textFilter, p.Width, p.Height)
	}

	return fmt.Sprintf("%s,%s,scale=%d:%d", aspectFilter, zoomFilter, p.Width, p.Height)
}
