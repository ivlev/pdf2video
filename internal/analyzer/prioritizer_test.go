package analyzer

import (
	"image"
	"testing"
)

func TestBlockPrioritizer_Prioritize(t *testing.T) {
	prioritizer := NewBlockPrioritizer()

	blocks := []Block{
		{
			Rect:  image.Rect(100, 700, 400, 750), // Footer-like
			Type:  BlockTypeFooter,
			Score: 0.5,
		},
		{
			Rect:  image.Rect(100, 50, 400, 100), // Header-like
			Type:  BlockTypeHeader,
			Score: 0.8,
		},
		{
			Rect:  image.Rect(200, 200, 800, 600), // Large content
			Type:  BlockTypeChart,
			Score: 0.9,
		},
	}

	prioritized := prioritizer.Prioritize(blocks, 1000, 800)

	if len(prioritized) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(prioritized))
	}

	// Header should be first (High content type score + high position score)
	if prioritized[0].Type != BlockTypeHeader {
		t.Errorf("Expected first block to be Header, got %s", prioritized[0].Type)
	}

	// Footer should be last
	if prioritized[2].Type != BlockTypeFooter {
		t.Errorf("Expected last block to be Footer, got %s", prioritized[2].Type)
	}
}

func TestEnhancedDetector_HeaderFooterClassification(t *testing.T) {
	// Simple smoke test for classification logic
	detector := NewEnhancedDetector()
	pageBounds := image.Rect(0, 0, 1000, 1000)

	// Mock metrics for a wide top block
	metrics := BlockMetrics{
		AspectRatio: 5.0,
		EdgeDensity: 0.1,
	}

	headerType := detector.classifyBlock(metrics, image.Rect(100, 20, 900, 60), pageBounds)
	if headerType != BlockTypeHeader {
		t.Errorf("Expected Header classification, got %s", headerType)
	}

	footerType := detector.classifyBlock(metrics, image.Rect(100, 950, 900, 980), pageBounds)
	if footerType != BlockTypeFooter {
		t.Errorf("Expected Footer classification, got %s", footerType)
	}
}
