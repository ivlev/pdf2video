package config

type Config struct {
	InputPath      string
	OutputVideo    string
	TotalDuration  float64
	Width          int
	Height         int
	FPS            int
	Workers        int
	FadeDuration   float64
	TransitionType string
	ZoomMode       string
	ZoomSpeed      float64
	DPI            int
	AudioPath      string
	Preset         string
	PageDurations  []float64
	VideoEncoder   string
	Quality        int
	ShowStats      bool
	BuildVersion   string
}

type SegmentParams struct {
	Width, Height int
	FPS           int
	Duration      float64
	ZoomMode      string
	ZoomSpeed     float64
	FadeDuration  float64
	PageIndex     int
}
