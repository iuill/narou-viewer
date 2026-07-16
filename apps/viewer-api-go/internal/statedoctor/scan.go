package statedoctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/state/bookmarks"
	"narou-viewer/apps/viewer-api-go/internal/state/novelsettings"
	"narou-viewer/apps/viewer-api-go/internal/state/preferences"
	"narou-viewer/apps/viewer-api-go/internal/state/readingstate"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/statesecurity"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

const (
	repairJobIndex         = "job_index_rebuild"
	repairCharacterProfile = "character_profile_rebuild"
	repairReaderSearch     = "reader_search_rebuild"
)

type scannedFile struct {
	path     string
	raw      []byte
	result   schemaguard.Result
	exists   bool
	accepted bool
}

type scanner struct {
	dataDir          string
	stateDir         string
	novelFetcherDir  string
	report           Report
	yamlFiles        map[string]scannedFile
	jsonFiles        map[string]scannedFile
	libraryNovelIDs  map[string]bool
	libraryReadable  bool
	excludedTopLevel map[string]bool
}

func Scan(ctx context.Context, dataDir string) (Report, error) {
	return ScanWithOptions(ctx, dataDir, ScanOptions{})
}

type ScanOptions struct {
	ExcludedTopLevel []string
}

func ScanWithOptions(ctx context.Context, dataDir string, options ScanOptions) (Report, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return Report{}, errors.New("data directory is required")
	}
	dataDir = filepath.Clean(dataDir)
	info, err := os.Stat(dataDir)
	if err != nil {
		return Report{}, err
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("data path is not a directory: %s", dataDir)
	}
	excludedTopLevel := map[string]bool{}
	for _, name := range options.ExcludedTopLevel {
		if name == "" || name == "." || filepath.Base(name) != name {
			return Report{}, fmt.Errorf("state doctor exclusion must be a top-level base name: %q", name)
		}
		excludedTopLevel[name] = true
	}
	s := &scanner{
		dataDir:          dataDir,
		stateDir:         filepath.Join(dataDir, "state"),
		novelFetcherDir:  filepath.Join(dataDir, "novel-fetcher"),
		report:           Report{DataDir: dataDir, Findings: []Finding{}},
		yamlFiles:        map[string]scannedFile{},
		jsonFiles:        map[string]scannedFile{},
		libraryNovelIDs:  map[string]bool{},
		excludedTopLevel: excludedTopLevel,
	}
	s.scanFileInventory()
	s.scanSensitiveState()
	s.scanSQLite(ctx)
	s.scanReconciliation()
	s.scanSensitivePlacement()
	s.report.finalize()
	return s.report, nil
}

func (s *scanner) scanFileInventory() {
	for _, item := range []struct {
		name     string
		contract schemaguard.Contract
	}{
		{name: readingstate.FileName, contract: readingstate.SchemaContract},
		{name: bookmarks.FileName, contract: bookmarks.SchemaContract},
		{name: preferences.FileName, contract: preferences.SchemaContract},
		{name: novelsettings.FileName, contract: novelsettings.SchemaContract},
		{name: aisettings.FileName, contract: aisettings.SchemaContract},
		{name: publications.FileName, contract: publications.SchemaContract},
	} {
		s.scanYAML(filepath.Join(s.stateDir, item.name), item.contract, true)
	}
	for _, item := range []struct {
		pattern  string
		contract schemaguard.Contract
	}{
		{pattern: filepath.Join(s.stateDir, "character_events", "*.yaml"), contract: characters.CharacterEventsSchemaContract},
		{pattern: filepath.Join(s.stateDir, "character_profiles", "*.yaml"), contract: characters.CharacterProfilesSchemaContract},
		{pattern: filepath.Join(s.stateDir, "term_profiles", "*.yaml"), contract: terms.SchemaContract},
		{pattern: filepath.Join(s.stateDir, "extraction_jobs", "*.yaml"), contract: extraction.JobSchemaContract},
		{pattern: filepath.Join(s.stateDir, "extraction_jobs", "index", "*.yaml"), contract: extraction.JobIndexSchemaContract},
	} {
		s.scanYAMLGlob(item.pattern, item.contract)
	}
	s.scanJSONGlob(filepath.Join(s.stateDir, "extraction_jobs", "checkpoints", "*.json"), checkpointstore.SchemaContract)
}

func (s *scanner) scanYAMLGlob(pattern string, contract schemaguard.Contract) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(pattern), Kind: "inventory_error", Severity: SeverityError, Observed: "glob failed", Supported: supportedVersions(contract), RecoveryHint: "path pattern と filesystem を確認してください。"})
		return
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(pattern), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: supportedVersions(contract), RecoveryHint: "per-novel state が未生成なら正常です。"})
		return
	}
	for _, path := range paths {
		s.scanYAML(path, contract, false)
	}
}

func (s *scanner) scanJSONGlob(pattern string, contract schemaguard.Contract) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(pattern), Kind: "inventory_error", Severity: SeverityError, Observed: "glob failed", Supported: supportedVersions(contract), RecoveryHint: "path pattern と filesystem を確認してください。"})
		return
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(pattern), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: supportedVersions(contract), RecoveryHint: "checkpoint がなければ正常です。"})
		return
	}
	for _, path := range paths {
		if s.rejectSymlink(contract.ID, path, supportedVersions(contract)) {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			s.add(Finding{SchemaID: contract.ID, Path: s.rel(path), Kind: "read_error", Severity: SeverityError, Observed: "unreadable", Supported: supportedVersions(contract), RecoveryHint: "権限と storage を確認し、元 file を変更せず復旧してください。"})
			continue
		}
		result, guardErr := schemaguard.CheckJSON(raw, contract.WithPath(s.rel(path)))
		s.jsonFiles[path] = scannedFile{path: path, raw: raw, result: result, exists: true, accepted: guardErr == nil}
		s.addSchemaResult(path, result, guardErr)
	}
}

func (s *scanner) scanYAML(path string, contract schemaguard.Contract, singleton bool) {
	if s.rejectSymlink(contract.ID, path, supportedVersions(contract)) {
		return
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.yamlFiles[path] = scannedFile{path: path}
		hint := "per-novel state が未生成なら正常です。"
		if singleton {
			hint = "新規環境では対応 service の初期化で生成されます。"
		}
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(path), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: supportedVersions(contract), RecoveryHint: hint})
		return
	}
	if err != nil {
		s.add(Finding{SchemaID: contract.ID, Path: s.rel(path), Kind: "read_error", Severity: SeverityError, Observed: "unreadable", Supported: supportedVersions(contract), RecoveryHint: "権限と storage を確認し、元 file を変更せず復旧してください。"})
		return
	}
	result, guardErr := schemaguard.CheckYAML(raw, contract.WithPath(s.rel(path)))
	s.yamlFiles[path] = scannedFile{path: path, raw: raw, result: result, exists: true, accepted: guardErr == nil}
	s.addSchemaResult(path, result, guardErr)
}

func (s *scanner) rejectSymlink(schemaID string, path string, supported string) bool {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "symlink_not_scanned", Severity: SeverityError, Observed: "symlink", Supported: supported, RecoveryHint: "link 先を読まず、data tree 内の regular file として復旧してください。"})
	return true
}

func (s *scanner) addSchemaResult(path string, result schemaguard.Result, err error) {
	severity := SeverityInfo
	kind := "schema_current"
	hint := "対応中の schema です。"
	if err == nil && result.Status == schemaguard.StatusLegacy {
		severity = SeverityWarning
		kind = "schema_legacy"
		hint = "対応 build で明示 migration または次回保存を行い、backup を更新してください。"
	} else if err != nil {
		severity = SeverityError
		kind = "schema_" + result.Status.String()
		hint = "対応する新しい build または supported backup を使い、現行 build で normalize / delete しないでください。"
	}
	s.add(Finding{
		SchemaID:     result.Contract.ID,
		Path:         s.rel(path),
		Kind:         kind,
		Severity:     severity,
		Observed:     observedVersion(result),
		Supported:    supportedVersions(result.Contract),
		RecoveryHint: hint,
	})
}

func (s *scanner) scanSensitiveState() {
	path := filepath.Join(s.stateDir, aisettings.FileName)
	if found, exists, err := statesecurity.HasLegacyPlaintextAPIKeyIfExists(path); err != nil {
		s.add(Finding{SchemaID: "VA-AI-SETTINGS", Path: s.rel(path), Kind: "plaintext_credential_scan_error", Severity: SeverityError, Observed: "unreadable", RecoveryHint: "raw YAML を変更せず、構文と権限を確認してください。"})
	} else if exists && found {
		s.add(Finding{SchemaID: "VA-AI-SETTINGS", Path: s.rel(path), Kind: "legacy_plaintext_api_key", Severity: SeverityError, Observed: "non-empty api_key", Supported: "encrypted credential", RecoveryHint: "対応 build と master passphrase で暗号化 migration し、再検査してください。値を log や archive manifest に出さないでください。"})
	}
	if versions, exists, err := statesecurity.APIKeyVersionsIfExists(path); err != nil {
		s.add(Finding{SchemaID: "VA-AI-SETTINGS-CRYPTO", Path: s.rel(path), Kind: "crypto_version_scan_error", Severity: SeverityError, Observed: "unreadable", Supported: "0,1", RecoveryHint: "raw YAML を変更せず api_key_version の型と schema を確認してください。"})
	} else if exists && len(versions) > 0 {
		observed := make([]string, 0, len(versions))
		unsupported := false
		for _, version := range versions {
			observed = append(observed, strconv.Itoa(version))
			if version != 0 && version != aisettings.APIKeyCryptoVersion {
				unsupported = true
			}
		}
		finding := Finding{SchemaID: "VA-AI-SETTINGS-CRYPTO", Path: s.rel(path), Kind: "crypto_current", Severity: SeverityInfo, Observed: strings.Join(observed, ","), Supported: "0,1", RecoveryHint: "API key ciphertext version は current です。"}
		if unsupported {
			finding.Kind = "crypto_future_unknown"
			finding.Severity = SeverityError
			finding.RecoveryHint = "対応 build または supported backup を使い、現行 build で再暗号化・消去しないでください。"
		}
		s.add(finding)
	}
	for _, item := range []struct {
		id   string
		path string
	}{
		{id: "VA-AI-SETTINGS", path: path},
		{id: "VA-AI-USAGE", path: filepath.Join(s.stateDir, "ai_usage.sqlite")},
		{id: "VA-READER-SEARCH", path: filepath.Join(s.stateDir, readertextcache.FileName)},
	} {
		s.scanSensitiveMode(item.id, item.path)
	}
}

func (s *scanner) scanSensitiveMode(schemaID string, path string) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "mode_check_error", Severity: SeverityError, Observed: "unreadable", Supported: "0600", RecoveryHint: "所有者と filesystem 権限を確認してください。"})
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sensitive_symlink", Severity: SeverityError, Observed: "symlink", Supported: "regular file 0600", RecoveryHint: "機微 file を data/state 内の regular file として配置し、link 先の露出を確認してください。"})
		return
	}
	if info.Mode().Perm() != 0o600 {
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "insecure_file_mode", Severity: SeverityWarning, Observed: fmt.Sprintf("%04o", info.Mode().Perm()), Supported: "0600", RecoveryHint: "owner-only の 0600 に変更し、backup / host mount 側 ACL も確認してください。"})
	}
}

func (s *scanner) scanSensitivePlacement() {
	expected := map[string]string{
		filepath.Clean(filepath.Join(s.stateDir, aisettings.FileName)):      "VA-AI-SETTINGS",
		filepath.Clean(filepath.Join(s.stateDir, "ai_usage.sqlite")):        "VA-AI-USAGE",
		filepath.Clean(filepath.Join(s.stateDir, readertextcache.FileName)): "VA-READER-SEARCH",
	}
	names := map[string]string{
		aisettings.FileName:      "VA-AI-SETTINGS",
		"ai_usage.sqlite":        "VA-AI-USAGE",
		readertextcache.FileName: "VA-READER-SEARCH",
	}
	_ = filepath.WalkDir(s.dataDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if s.excludedTopLevelPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		schemaID, sensitive := names[entry.Name()]
		if !sensitive || expected[filepath.Clean(path)] != "" {
			return nil
		}
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sensitive_file_outside_state", Severity: SeverityWarning, Observed: "unexpected placement", Supported: "state/" + entry.Name(), RecoveryHint: "意図した backup / quarantine か確認し、不要な複製を安全に廃棄してください。内容を表示しないでください。"})
		return nil
	})
}

func (s *scanner) excludedTopLevelPath(path string) bool {
	relative, err := filepath.Rel(s.dataDir, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return false
	}
	first := strings.SplitN(filepath.ToSlash(relative), "/", 2)[0]
	return s.excludedTopLevel[first]
}

func (s *scanner) add(finding Finding) {
	if finding.ID == "" {
		finding.ID = findingID(finding.SchemaID, finding.Kind, finding.Path, finding.RepairTarget)
	}
	s.report.Findings = append(s.report.Findings, finding)
}

func (s *scanner) rel(path string) string {
	relative, err := filepath.Rel(s.dataDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func findingID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "finding-" + hex.EncodeToString(sum[:8])
}

func observedVersion(result schemaguard.Result) string {
	if result.Observed == nil {
		if result.Status == schemaguard.StatusMalformed {
			return "malformed"
		}
		return "missing"
	}
	return strconv.Itoa(*result.Observed)
}

func supportedVersions(contract schemaguard.Contract) string {
	versions := append([]int{}, contract.ReadableLegacy...)
	versions = append(versions, contract.Current)
	sort.Ints(versions)
	parts := make([]string, 0, len(versions))
	for _, version := range versions {
		parts = append(parts, strconv.Itoa(version))
	}
	return strings.Join(parts, ",")
}
