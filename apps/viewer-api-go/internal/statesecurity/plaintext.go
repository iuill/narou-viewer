package statesecurity

import (
	"errors"
	"fmt"
	"os"
	"sort"
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

func APIKeyVersionsIfExists(path string) ([]int, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	versions, err := APIKeyVersions(raw)
	return versions, true, err
}

func APIKeyVersions(raw []byte) ([]int, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	versions := map[int]bool{}
	if err := collectAPIKeyVersions(&document, versions); err != nil {
		return nil, err
	}
	result := make([]int, 0, len(versions))
	for version := range versions {
		result = append(result, version)
	}
	sort.Ints(result)
	return result, nil
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

func collectAPIKeyVersions(node *yaml.Node, versions map[int]bool) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if key.Value == "api_key_version" {
				var version int
				if value.Kind != yaml.ScalarNode || value.Decode(&version) != nil {
					return fmt.Errorf("api_key_version must be an integer")
				}
				versions[version] = true
			}
			if err := collectAPIKeyVersions(value, versions); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := collectAPIKeyVersions(child, versions); err != nil {
			return err
		}
	}
	return nil
}
