package extraction

import "narou-viewer/apps/viewer-api-go/internal/state/schemaguard"

const (
	jobSchemaVersion      = 2
	jobIndexSchemaVersion = 2
)

var JobSchemaContract = schemaguard.Contract{
	ID:            "VA-EXTRACTION-JOBS",
	Path:          "extraction_jobs/*.yaml",
	Current:       jobSchemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}

var JobIndexSchemaContract = schemaguard.Contract{
	ID:            "VA-EXTRACTION-INDEX",
	Path:          "extraction_jobs/index/*.yaml",
	Current:       jobIndexSchemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}
