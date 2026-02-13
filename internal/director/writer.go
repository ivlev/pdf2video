package director

import (
	"os"

	"gopkg.in/yaml.v3"
)

// WriteScenario writes a scenario to a YAML file
func WriteScenario(scenario *Scenario, path string) error {
	data, err := yaml.Marshal(scenario)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ReadScenario reads a scenario from a YAML file
func ReadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var scenario Scenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		return nil, err
	}

	return &scenario, nil
}
