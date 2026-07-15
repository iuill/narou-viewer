package statesecurity

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func HasLegacyPlaintextAPIKey(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false, err
	}
	return containsNonEmptyAPIKey(&root), nil
}

func HasLegacyPlaintextAPIKeyIfExists(path string) (bool, bool, error) {
	value, err := HasLegacyPlaintextAPIKey(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	return value, true, err
}

func containsNonEmptyAPIKey(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if strings.EqualFold(strings.TrimSpace(key.Value), "api_key") && yamlNodeHasNonEmptyScalar(value) {
				return true
			}
			if containsNonEmptyAPIKey(value) {
				return true
			}
		}
		return false
	}
	for _, child := range node.Content {
		if containsNonEmptyAPIKey(child) {
			return true
		}
	}
	return false
}

func yamlNodeHasNonEmptyScalar(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return false
	}
	return strings.TrimSpace(node.Value) != ""
}
