package extraction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/fsatomic"

	"gopkg.in/yaml.v3"
)

func readYAMLIfExists(path string, target any) (bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, yaml.Unmarshal(raw, target)
}

func removeIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func writeYAMLAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := marshalYAML(value)
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(path, raw, 0o644)
}

func marshalYAML(value any) (raw []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("marshal yaml: %v", recovered)
		}
	}()
	return yaml.Marshal(value)
}
