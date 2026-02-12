package director

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateScenarioPath(t *testing.T) {
	path := GenerateScenarioPath()

	if !filepath.IsAbs(path) {
		// Convert to absolute for checking
		absPath, _ := filepath.Abs(path)
		path = absPath
	}

	// Should contain timestamp format
	if !contains(path, "scenario_") {
		t.Errorf("Path should contain 'scenario_': %s", path)
	}

	if !contains(path, "internal/scenarios") {
		t.Errorf("Path should be in internal/scenarios: %s", path)
	}

	t.Logf("Generated path: %s", path)
}

func TestFindLatestScenario(t *testing.T) {
	// Create test scenarios directory
	testDir := filepath.Join("internal", "scenarios")
	os.MkdirAll(testDir, 0755)

	// Create test files with different timestamps
	files := []string{
		filepath.Join(testDir, "scenario_2026-02-12_10-00-00.yaml"),
		filepath.Join(testDir, "scenario_2026-02-13_01-00-00.yaml"),
		filepath.Join(testDir, "scenario_2026-02-11_15-30-00.yaml"),
	}

	for i, f := range files {
		os.WriteFile(f, []byte("test"), 0644)
		// Set different modification times
		modTime := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(f, modTime, modTime)
	}

	defer func() {
		for _, f := range files {
			os.Remove(f)
		}
	}()

	latest, err := FindLatestScenario()
	if err != nil {
		t.Fatalf("FindLatestScenario failed: %v", err)
	}

	t.Logf("Latest scenario: %s", latest)

	// Should be the last file (most recent mod time)
	if !contains(latest, files[len(files)-1]) {
		t.Errorf("Expected latest to be %s, got %s", files[len(files)-1], latest)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
