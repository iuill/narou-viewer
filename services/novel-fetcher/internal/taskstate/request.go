package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	targetNcodePattern       = regexp.MustCompile(`(?i)n\d+[a-z]+`)
	targetKakuyomuPathRegexp = regexp.MustCompile(`^/works/(\d+)(?:/.*)?$`)
)

type Request struct {
	Kind    string         `json:"kind"`
	Target  string         `json:"target,omitempty"`
	WorkID  int            `json:"work_id,omitempty"`
	Options RequestOptions `json:"options"`
}

type RequestOptions struct {
	Force           bool `json:"force"`
	ForceRedownload bool `json:"force_redownload"`
	SkipUnchanged   bool `json:"skip_unchanged"`
}

func RequestForTask(task *Task) (Request, string, string, error) {
	request := Request{
		Kind:   task.Kind,
		Target: task.Target,
		WorkID: task.WorkID,
		Options: RequestOptions{
			Force:           task.Force,
			ForceRedownload: task.ForceRedownload,
			SkipUnchanged:   task.SkipUnchanged,
		},
	}
	canonicalTarget, err := validateRequest(request)
	if err != nil {
		return Request{}, "", "", fmt.Errorf("task %q: %w", task.ID, err)
	}
	fingerprintInput := struct {
		Kind            string         `json:"kind"`
		CanonicalTarget string         `json:"canonical_target"`
		WorkID          int            `json:"work_id,omitempty"`
		Options         RequestOptions `json:"options"`
	}{
		Kind:            request.Kind,
		CanonicalTarget: canonicalTarget,
		WorkID:          request.WorkID,
		Options:         request.Options,
	}
	requestJSON, err := json.Marshal(fingerprintInput)
	if err != nil {
		return Request{}, "", "", err
	}
	fingerprintBytes := sha256.Sum256(requestJSON)
	return request, canonicalTarget, hex.EncodeToString(fingerprintBytes[:]), nil
}

func DecodeRequest(raw string) (Request, error) {
	var request Request
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return Request{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Request{}, fmt.Errorf("task request contains trailing JSON")
		}
		return Request{}, err
	}
	if _, err := validateRequest(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func validateRequest(request Request) (string, error) {
	switch request.Kind {
	case "download":
		if request.Options.ForceRedownload || request.Options.SkipUnchanged {
			return "", fmt.Errorf("download task request contains unsupported update options")
		}
		if request.WorkID < 0 {
			return "", fmt.Errorf("download task request contains an invalid work id")
		}
		canonicalTarget := CanonicalTarget(request.Target)
		if canonicalTarget == "" {
			return "", fmt.Errorf("download task request contains an empty target")
		}
		return canonicalTarget, nil
	case "update":
		if request.Target != "" || request.WorkID <= 0 {
			return "", fmt.Errorf("update task request must contain one work id and no target")
		}
		if request.Options.Force {
			return "", fmt.Errorf("update task request contains unsupported download options")
		}
		return "work:" + strconv.Itoa(request.WorkID), nil
	case "resume":
		if request.Target != "" || request.WorkID <= 0 {
			return "", fmt.Errorf("resume task request must contain one work id and no target")
		}
		if request.Options.Force || request.Options.ForceRedownload || request.Options.SkipUnchanged {
			return "", fmt.Errorf("resume task request contains unsupported options")
		}
		return "work:" + strconv.Itoa(request.WorkID), nil
	default:
		return "", fmt.Errorf("unsupported task request kind %q", request.Kind)
	}
}

// CanonicalTarget returns the durable identity used to reserve a download
// target. It intentionally collapses accepted aliases such as an N code and
// any episode URL for the same work while preserving the original request for
// execution.
func CanonicalTarget(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "" {
		return ""
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		if host == "ncode.syosetu.com" {
			if match := targetNcodePattern.FindString(parsed.Path); match != "" {
				return "site:syosetu:" + strings.ToLower(match)
			}
		}
		if host == "kakuyomu.jp" {
			if match := targetKakuyomuPathRegexp.FindStringSubmatch(parsed.Path); len(match) >= 2 {
				return "site:kakuyomu:" + match[1]
			}
		}
		parsed.Fragment = ""
		parsed.RawQuery = ""
		parsed.Path = path.Clean(parsed.Path)
		return "url:" + strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + parsed.EscapedPath()
	}
	if match := targetNcodePattern.FindString(target); match != "" {
		return "site:syosetu:" + strings.ToLower(match)
	}
	return "url:" + strings.ToLower(strings.TrimSuffix(target, "/"))
}
