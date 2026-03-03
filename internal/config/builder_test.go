package config

import (
	"testing"
)

func TestConfigBuilder_Presets(t *testing.T) {
	builder := NewBuilder("test-version")

	// Test 9:16 preset
	cfg, err := builder.Build([]string{"-preset", "9:16", "-input", "test.pdf"})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.Width != 720 || cfg.Height != 1280 {
		t.Errorf("Expected 720x1280 for 9:16 preset, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestConfigBuilder_ManualOverrides(t *testing.T) {
	builder := NewBuilder("test-version")

	cfg, err := builder.Build([]string{
		"-width", "1920",
		"-height", "1080",
		"-fps", "60",
		"-input", "test.pdf",
		"-output", "custom.mp4",
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.Width != 1920 || cfg.Height != 1080 {
		t.Errorf("Expected 1920x1080, got %dx%d", cfg.Width, cfg.Height)
	}

	if cfg.FPS != 60 {
		t.Errorf("Expected 60 FPS, got %d", cfg.FPS)
	}

	if cfg.OutputVideo != "custom.mp4" {
		t.Errorf("Expected custom.mp4, got %s", cfg.OutputVideo)
	}
}

func TestConfigBuilder_BlackScreenTransition(t *testing.T) {
	builder := NewBuilder("test-version")

	// If black-screen-transition is empty, it should inherit from transition
	cfg, err := builder.Build([]string{"-transition", "wipeleft", "-input", "test.pdf"})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.BlackScreenTransition != "wipeleft" {
		t.Errorf("Expected BlackScreenTransition to be wipeleft, got %s", cfg.BlackScreenTransition)
	}

	// Manual override
	builder2 := NewBuilder("test-version")
	cfg2, err := builder2.Build([]string{
		"-transition", "fade",
		"-black-screen-transition", "none",
		"-input", "test.pdf",
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg2.BlackScreenTransition != "none" {
		t.Errorf("Expected BlackScreenTransition to be none, got %s", cfg2.BlackScreenTransition)
	}
}
func TestConfigBuilder_Stats(t *testing.T) {
	builder := NewBuilder("test-version")

	cfg, err := builder.Build([]string{"-stats", "-input", "test.pdf"})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !cfg.ShowStats {
		t.Errorf("Expected ShowStats to be true, got false")
	}
}
