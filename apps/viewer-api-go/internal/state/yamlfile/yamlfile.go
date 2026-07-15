package yamlfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"

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

func ReadGuarded(path string, contract schemaguard.Contract, target any) (schemaguard.Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return schemaguard.Result{}, err
	}
	contract = contract.WithPath(path)
	result, err := schemaguard.CheckYAML(raw, contract)
	if err != nil {
		return result, err
	}
	if err := yaml.Unmarshal(raw, target); err != nil {
		return schemaguard.Malformed(contract, err)
	}
	return result, nil
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
