package director

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GenerateScenarioPath creates a timestamped scenario filename
func GenerateScenarioPath() string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return filepath.Join("internal", "scenarios", fmt.Sprintf("scenario_%s.yaml", timestamp))
}

// FindLatestScenario finds the most recent scenario file in the scenarios directory
func FindLatestScenario() (string, error) {
	scenariosDir := filepath.Join("internal", "scenarios")

	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		return "", fmt.Errorf("failed to read scenarios directory: %w", err)
	}

	var scenarios []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			scenarios = append(scenarios, filepath.Join(scenariosDir, entry.Name()))
		}
	}

	if len(scenarios) == 0 {
		return "", fmt.Errorf("no scenario files found in %s", scenariosDir)
	}

	// Sort by modification time (newest first)
	sort.Slice(scenarios, func(i, j int) bool {
		infoI, _ := os.Stat(scenarios[i])
		infoJ, _ := os.Stat(scenarios[j])
		return infoI.ModTime().After(infoJ.ModTime())
	})

	return scenarios[0], nil
}
