package schemaguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type MissingPolicy int

const (
	MissingReject MissingPolicy = iota
	MissingTreatAsLegacy
)

type Status int

const (
	StatusCurrent Status = iota
	StatusLegacy
	StatusFutureUnknown
	StatusUnsupportedLegacy
	StatusMalformed
)

func (s Status) String() string {
	switch s {
	case StatusCurrent:
		return "current"
	case StatusLegacy:
		return "legacy"
	case StatusFutureUnknown:
		return "future_unknown"
	case StatusUnsupportedLegacy:
		return "unsupported_legacy"
	case StatusMalformed:
		return "malformed"
	default:
		return "unknown"
	}
}

type Contract struct {
	ID                   string
	Path                 string
	Current              int
	ReadableLegacy       []int
	MissingPolicy        MissingPolicy
	MissingLegacyVersion int
}

func (c Contract) WithPath(path string) Contract {
	if strings.TrimSpace(c.Path) == "" {
		c.Path = path
	}
	return c
}

type Result struct {
	Contract Contract
	Status   Status
	Observed *int
}

type GuardError struct {
	Result Result
	Cause  error
}

func (e *GuardError) Error() string {
	path := e.Result.Contract.Path
	if path == "" {
		path = "unknown path"
	}
	if e.Cause != nil {
		return fmt.Sprintf("invalid %s state at %q: %v", e.Result.Contract.ID, path, e.Cause)
	}
	observed := "missing"
	if e.Result.Observed != nil {
		observed = strconv.Itoa(*e.Result.Observed)
	}
	return fmt.Sprintf(
		"unsupported %s schema at %q: observed %s, supported current %d; use a compatible build or restore a supported backup",
		e.Result.Contract.ID,
		path,
		observed,
		e.Result.Contract.Current,
	)
}

func (e *GuardError) Unwrap() error {
	return e.Cause
}

func CheckYAML(document []byte, contract Contract) (Result, error) {
	var header struct {
		SchemaVersion *int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(document, &header); err != nil {
		return malformedResult(contract, err)
	}
	return classify(header.SchemaVersion, contract)
}

func CheckJSON(document []byte, contract Contract) (Result, error) {
	var header struct {
		SchemaVersion *int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(document, &header); err != nil {
		return malformedResult(contract, err)
	}
	return classify(header.SchemaVersion, contract)
}

func classify(observed *int, contract Contract) (Result, error) {
	result := Result{Contract: contract, Observed: observed}
	if observed == nil {
		if contract.MissingPolicy == MissingTreatAsLegacy && slices.Contains(contract.ReadableLegacy, contract.MissingLegacyVersion) {
			result.Status = StatusLegacy
			return result, nil
		}
		result.Status = StatusUnsupportedLegacy
		return result, &GuardError{Result: result}
	}
	if *observed == contract.Current {
		result.Status = StatusCurrent
		return result, nil
	}
	if slices.Contains(contract.ReadableLegacy, *observed) {
		result.Status = StatusLegacy
		return result, nil
	}
	if *observed > contract.Current {
		result.Status = StatusFutureUnknown
	} else {
		result.Status = StatusUnsupportedLegacy
	}
	return result, &GuardError{Result: result}
}

func malformedResult(contract Contract, cause error) (Result, error) {
	result := Result{Contract: contract, Status: StatusMalformed}
	return result, &GuardError{Result: result, Cause: cause}
}

func Malformed(contract Contract, cause error) (Result, error) {
	return malformedResult(contract, cause)
}

func AsGuardError(err error) (*GuardError, bool) {
	var guardError *GuardError
	if !errors.As(err, &guardError) {
		return nil, false
	}
	return guardError, true
}
