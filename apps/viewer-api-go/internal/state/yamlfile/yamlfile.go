package yamlfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/fsatomic"

	"gopkg.in/yaml.v3"
)

func Ensure(path string, value any) error {
	return EnsureMode(path, value, 0o644)
}

func EnsureMode(path string, value any, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return WriteAtomicMode(path, value, mode)
}

func Read(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, target)
}

func WriteAtomic(path string, value any) error {
	return WriteAtomicMode(path, value, 0o644)
}

func WriteAtomicMode(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := marshalYAML(value)
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(path, raw, mode)
}

func marshalYAML(value any) (raw []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("marshal yaml: %v", recovered)
		}
	}()
	return yaml.Marshal(value)
}
