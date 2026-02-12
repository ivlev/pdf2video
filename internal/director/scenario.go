package director

// Scenario represents a complete animation scenario for a video
type Scenario struct {
	Version string  `yaml:"version"`
	Slides  []Slide `yaml:"slides"`
}

// Slide represents a single page/image with its animation keyframes
type Slide struct {
	ID        int        `yaml:"id"`
	Input     string     `yaml:"input"`
	Duration  float64    `yaml:"duration"` // Total duration in seconds
	Keyframes []Keyframe `yaml:"keyframes"`
}

// Keyframe represents a camera position at a specific time
type Keyframe struct {
	Time  float64   `yaml:"time"`  // Time offset in seconds
	Focus string    `yaml:"focus"` // Description of focus region
	Rect  Rectangle `yaml:"rect"`  // Target rectangle
	Zoom  float64   `yaml:"zoom"`  // Zoom level (1.0 = no zoom)
}

// Rectangle represents a bounding box
type Rectangle struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
	W int `yaml:"w"`
	H int `yaml:"h"`
}
