package statebackup

import (
	"strings"
	"testing"
)

func TestValidateBackupArchiveContractRejectsInvalidLayouts(t *testing.T) {
	validManifest := validEmptyManifest("contract-test")
	valid := sourceFile{record: FileRecord{
		Path:  "state/extraction_jobs/job-1.yaml",
		Group: GroupVAExtraction,
		Size:  1,
		Mode:  0o600,
	}}
	for name, mutate := range map[string]func(*[]sourceFile, *Manifest){
		"prepopulated_manifest": func(_ *[]sourceFile, manifest *Manifest) {
			manifest.Files = []FileRecord{{Path: valid.record.Path}}
		},
		"group_mismatch": func(files *[]sourceFile, _ *Manifest) {
			(*files)[0].record.Group = GroupVACore
		},
		"duplicate": func(files *[]sourceFile, _ *Manifest) {
			*files = append(*files, (*files)[0])
		},
		"unsafe_header": func(files *[]sourceFile, _ *Manifest) {
			(*files)[0].record.Path = "state/extraction_jobs/../extraction_jobs/job-1.yaml"
		},
		"negative_size": func(files *[]sourceFile, _ *Manifest) {
			(*files)[0].record.Size = -1
		},
		"payload_too_large": func(files *[]sourceFile, _ *Manifest) {
			(*files)[0].record.Size = maxUncompressedArchiveBytes
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := []sourceFile{valid}
			manifest := validManifest
			mutate(&files, &manifest)
			if err := validateBackupArchiveContract(files, manifest); err == nil {
				t.Fatal("invalid backup archive layout was accepted")
			}
		})
	}
}

func TestArchiveBudgetRejectsEntryCountAndUnsafeManifestName(t *testing.T) {
	budget := archiveBudget{entries: maxArchiveFiles}
	if err := budget.add(ManifestName, 0); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("entry count error = %v", err)
	}
	if err := (&archiveBudget{}).add("../manifest.json", 1); err == nil {
		t.Fatal("archive budget accepted an unsafe entry name")
	}
}
