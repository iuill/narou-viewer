package characters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/state/filequarantine"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"

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

func readCharacterEventsIfExists(path string, target *characterEventsDocument) (bool, schemaguard.Result, error) {
	return readGuardedYAMLIfExists(path, CharacterEventsSchemaContract, target)
}

func readCharacterProfilesIfExists(path string, target *profilesDocument) (bool, schemaguard.Result, error) {
	return readGuardedYAMLIfExists(path, CharacterProfilesSchemaContract, target)
}

func readGuardedYAMLIfExists(path string, contract schemaguard.Contract, target any) (bool, schemaguard.Result, error) {
	result, err := yamlfile.ReadGuarded(path, contract, target)
	if errors.Is(err, os.ErrNotExist) {
		return false, schemaguard.Result{}, nil
	}
	if err != nil {
		return false, result, err
	}
	return true, result, nil
}

func prepareCharacterProfileForWrite(path string) error {
	var doc profilesDocument
	_, _, err := readCharacterProfilesIfExists(path, &doc)
	if err == nil {
		return nil
	}
	if _, ok := schemaguard.AsGuardError(err); !ok {
		return err
	}
	_, err = filequarantine.Move(path, "unsupported")
	return err
}

func quarantineCharacterProfile(path string, err error) (bool, error) {
	if _, ok := schemaguard.AsGuardError(err); !ok {
		return false, err
	}
	_, moveErr := filequarantine.Move(path, "unsupported")
	return moveErr == nil, moveErr
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
