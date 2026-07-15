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
	return containsNonEmptyAPIKeyNode(node, map[*yaml.Node]bool{})
}

func containsNonEmptyAPIKeyNode(node *yaml.Node, visited map[*yaml.Node]bool) bool {
	if node == nil {
		return false
	}
	if visited[node] {
		return false
	}
	visited[node] = true
	if node.Kind == yaml.AliasNode {
		return containsNonEmptyAPIKeyNode(node.Alias, visited)
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if strings.EqualFold(strings.TrimSpace(yamlScalarValue(key)), "api_key") && yamlNodeHasNonEmptyScalar(value) {
				return true
			}
			if containsNonEmptyAPIKeyNode(value, visited) {
				return true
			}
		}
		return false
	}
	for _, child := range node.Content {
		if containsNonEmptyAPIKeyNode(child, visited) {
			return true
		}
	}
	return false
}

func yamlNodeHasNonEmptyScalar(node *yaml.Node) bool {
	node = resolvedYAMLNode(node, map[*yaml.Node]bool{})
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return false
	}
	return strings.TrimSpace(node.Value) != ""
}

func collectAPIKeyVersions(node *yaml.Node, versions map[int]bool) error {
	return collectAPIKeyVersionsNode(node, versions, map[*yaml.Node]bool{})
}

func collectAPIKeyVersionsNode(node *yaml.Node, versions map[int]bool, visited map[*yaml.Node]bool) error {
	if node == nil {
		return nil
	}
	if visited[node] {
		return nil
	}
	visited[node] = true
	if node.Kind == yaml.AliasNode {
		return collectAPIKeyVersionsNode(node.Alias, versions, visited)
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if yamlScalarValue(key) == "api_key_version" {
				var version int
				resolved := resolvedYAMLNode(value, map[*yaml.Node]bool{})
				if resolved == nil || resolved.Kind != yaml.ScalarNode || resolved.Decode(&version) != nil {
					return fmt.Errorf("api_key_version must be an integer")
				}
				versions[version] = true
			}
			if err := collectAPIKeyVersionsNode(value, versions, visited); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := collectAPIKeyVersionsNode(child, versions, visited); err != nil {
			return err
		}
	}
	return nil
}

func yamlScalarValue(node *yaml.Node) string {
	node = resolvedYAMLNode(node, map[*yaml.Node]bool{})
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func resolvedYAMLNode(node *yaml.Node, visited map[*yaml.Node]bool) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		if visited[node] {
			return nil
		}
		visited[node] = true
		node = node.Alias
	}
	return node
}
