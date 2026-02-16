package config

import "fmt"

type Config struct {
	InputPath        string
	OutputVideo      string
	TotalDuration    float64
	Width            int
	Height           int
	FPS              int
	Workers          int
	FadeDuration     float64
	TransitionType   string
	ZoomMode         string
	ZoomSpeed        float64
	DPI              int
	AudioPath        string
	Preset           string
	PageDurations    []float64
	VideoEncoder     string
	Quality          int
	ShowStats        bool
	BuildVersion     string
	AnalyzeMode      string
	MinBlockArea     int
	EdgeThreshold    float64
	GenerateScenario bool
	ScenarioOutput   string
	ScenarioInput    string
	OutroDuration    float64
	BackgroundAudio  string
	BackgroundVolume float64
}

type SegmentParams struct {
	Width, Height int
	FPS           int
	Duration      float64
	ZoomMode      string
	ZoomSpeed     float64
	FadeDuration  float64
	OutroDuration float64
	PageIndex     int
}

var SupportedTransitions = []string{
	"fade", "wipeleft", "wiperight", "wipeup", "wipedown",
	"slideleft", "slideright", "slideup", "slidedown",
	"circlecrop", "rectcrop", "distance", "fadeblack", "fadewhite",
	"radial", "smoothstep", "circularreveal", "pixelize", "dissolve", "none",
}

var SupportedZoomModes = []string{
	"center", "top-left", "top-right", "bottom-left", "bottom-right",
	"random", "out-center", "out-random",
}

func (c *Config) Validate() error {
	if c.Width <= 0 || c.Width%2 != 0 {
		return fmt.Errorf("width must be positive and even (got %d)", c.Width)
	}
	if c.Height <= 0 || c.Height%2 != 0 {
		return fmt.Errorf("height must be positive and even (got %d)", c.Height)
	}
	if c.FPS < 1 || c.FPS > 120 {
		return fmt.Errorf("fps must be between 1 and 120 (got %d)", c.FPS)
	}
	if c.DPI < 72 || c.DPI > 1200 {
		return fmt.Errorf("dpi must be between 72 and 1200 (got %d)", c.DPI)
	}
	if c.FadeDuration < 0 {
		return fmt.Errorf("fade duration cannot be negative")
	}
	if c.OutroDuration < 0 {
		return fmt.Errorf("outro duration cannot be negative")
	}
	if c.ZoomSpeed <= 0 {
		return fmt.Errorf("zoom-speed must be positive")
	}
	if c.Workers < 1 {
		return fmt.Errorf("workers must be at least 1")
	}

	// Validate TransitionType
	foundTransition := false
	for _, t := range SupportedTransitions {
		if c.TransitionType == t {
			foundTransition = true
			break
		}
	}
	if !foundTransition {
		return fmt.Errorf("unsupported transition type: %s. Supported: %v", c.TransitionType, SupportedTransitions)
	}

	// Validate ZoomMode
	foundZoom := false
	for _, z := range SupportedZoomModes {
		if c.ZoomMode == z {
			foundZoom = true
			break
		}
	}
	if !foundZoom {
		return fmt.Errorf("unsupported zoom mode: %s. Supported: %v", c.ZoomMode, SupportedZoomModes)
	}

	// Validate AnalyzeMode
	if c.AnalyzeMode != "contrast" && c.AnalyzeMode != "ocr" {
		return fmt.Errorf("unsupported analyze mode: %s. Use 'contrast' or 'ocr'", c.AnalyzeMode)
	}

	return nil
}
