package statebackup

import (
	"time"

	"filippo.io/age"

	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
)

const (
	ManifestFormatVersion = 1
	ManifestName          = "manifest.json"
	ArchiveSuffix         = ".tar.gz.age"
	GroupNFCanonical      = "NF-CANONICAL"
	GroupVACore           = "VA-CORE"
	GroupVAExtraction     = "VA-EXTRACTION"
	GroupVAHistory        = "VA-HISTORY"
	GroupVACache          = "VA-CACHE"
	GroupSecrets          = "SECRETS"
)

var requiredGroups = []string{GroupNFCanonical, GroupVACore, GroupVAExtraction, GroupVAHistory}

type GroupRecord struct {
	ID       string `json:"id"`
	Included bool   `json:"included"`
	Reason   string `json:"reason,omitempty"`
}

type SchemaRecord struct {
	SchemaID  string `json:"schema_id"`
	Path      string `json:"path"`
	Observed  string `json:"observed"`
	Supported string `json:"supported"`
	Status    string `json:"status"`
	Group     string `json:"group"`
}

type FileRecord struct {
	Path   string `json:"path"`
	Group  string `json:"group"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	FormatVersion    int                 `json:"format_version"`
	GenerationID     string              `json:"generation_id"`
	CreatedAt        string              `json:"created_at"`
	ApplicationBuild string              `json:"application_build"`
	SnapshotMethod   string              `json:"snapshot_method"`
	Encryption       string              `json:"encryption"`
	KeyReference     string              `json:"key_reference"`
	SecretReferences []string            `json:"secret_references"`
	Groups           []GroupRecord       `json:"groups"`
	Schemas          []SchemaRecord      `json:"schemas"`
	Files            []FileRecord        `json:"files"`
	DoctorSummary    statedoctor.Summary `json:"doctor_summary"`
}

type BackupOptions struct {
	DataDir          string
	OutputDir        string
	ApplicationBuild string
	KeyReference     string
	Recipient        age.Recipient
	Now              func() time.Time
	GenerationID     func() (string, error)
	Retention        *RetentionPolicy
}

type BackupResult struct {
	ArchivePath string
	Manifest    Manifest
	Pruned      []string
}

type RestoreOptions struct {
	DataDir              string
	ArchivePath          string
	KeyReference         string
	Identities           []age.Identity
	AllowInsecureArchive bool
}

type RestoreResult struct {
	Manifest Manifest
	Report   statedoctor.Report
}

type RetentionPolicy struct {
	KeepGenerations int
	MaxAge          time.Duration
	Now             func() time.Time
}
