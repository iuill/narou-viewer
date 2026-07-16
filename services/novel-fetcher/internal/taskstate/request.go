package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	Targets []string       `json:"targets,omitempty"`
	WorkIDs []int          `json:"work_ids,omitempty"`
	Options RequestOptions `json:"options"`
}

type RequestOptions struct {
	Force           bool `json:"force"`
	ForceRedownload bool `json:"force_redownload"`
	SkipUnchanged   bool `json:"skip_unchanged"`
}

func RequestForTask(task *Task) (Request, string, string, error) {
	request := Request{
		Kind:    task.Kind,
		Targets: append([]string{}, task.Targets...),
		WorkIDs: append([]int{}, task.NovelIDs...),
		Options: RequestOptions{
			Force:           task.Force,
			ForceRedownload: task.ForceRedownload,
			SkipUnchanged:   task.SkipUnchanged,
		},
	}
	canonicalTarget := ""
	if request.Kind != "download" && len(request.WorkIDs) > 0 {
		canonicalTarget = "work:" + strconv.Itoa(request.WorkIDs[0])
	} else if len(request.Targets) > 0 {
		canonicalTarget = CanonicalTarget(request.Targets[0])
	}
	if canonicalTarget == "" {
		return Request{}, "", "", fmt.Errorf("task %q has no target", task.ID)
	}
	fingerprintInput := struct {
		Kind            string         `json:"kind"`
		CanonicalTarget string         `json:"canonical_target"`
		WorkIDs         []int          `json:"work_ids,omitempty"`
		Options         RequestOptions `json:"options"`
	}{
		Kind:            request.Kind,
		CanonicalTarget: canonicalTarget,
		WorkIDs:         request.WorkIDs,
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
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return Request{}, err
	}
	if request.Kind == "" || (len(request.Targets) == 0 && len(request.WorkIDs) == 0) {
		return Request{}, fmt.Errorf("task request is missing kind or target")
	}
	return request, nil
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
