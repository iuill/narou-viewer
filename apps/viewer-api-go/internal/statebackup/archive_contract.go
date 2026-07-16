package statebackup

import (
	"encoding/json"
	"errors"
	"fmt"
)

const maxManifestBytes = 4 << 20
const maxArchiveFiles = 1_000_000
const maxUncompressedArchiveBytes int64 = 1 << 40

type archiveBudget struct {
	entries           int
	uncompressedBytes int64
}

func (budget *archiveBudget) add(name string, size int64) error {
	if size < 0 || !safeArchiveEntryName(name) {
		return fmt.Errorf("unsupported archive entry: %s", name)
	}
	if budget.entries >= maxArchiveFiles {
		return errors.New("archive contains too many entries")
	}
	if size > maxUncompressedArchiveBytes-budget.uncompressedBytes {
		return errors.New("archive uncompressed payload exceeds size limit")
	}
	if name == ManifestName && size > maxManifestBytes {
		return errors.New("manifest exceeds size limit")
	}
	budget.entries++
	budget.uncompressedBytes += size
	return nil
}

func validateBackupArchiveContract(files []sourceFile, manifest Manifest) error {
	if len(manifest.Files) != 0 {
		return errors.New("backup manifest files must be empty before archive creation")
	}
	preview := manifest
	preview.Files = make([]FileRecord, 0, len(files))
	budget := archiveBudget{}
	seen := map[string]bool{}
	for _, file := range files {
		record := file.record
		group, ok := groupForPayloadPath(record.Path)
		if !ok || group != record.Group {
			return fmt.Errorf("backup payload path does not match its consistency group: %s", record.Path)
		}
		name := "payload/" + record.Path
		if seen[name] {
			return fmt.Errorf("duplicate archive entry: %s", name)
		}
		seen[name] = true
		if err := budget.add(name, record.Size); err != nil {
			return err
		}
		record.SHA256 = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		preview.Files = append(preview.Files, record)
	}
	raw, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return err
	}
	return budget.add(ManifestName, int64(len(raw)+1))
}
