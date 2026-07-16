package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
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
		canonicalTarget = canonicalizeTarget(request.Targets[0])
	}
	if canonicalTarget == "" {
		return Request{}, "", "", fmt.Errorf("task %q has no target", task.ID)
	}
	requestJSON, err := json.Marshal(request)
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

func canonicalizeTarget(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "" {
		return ""
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Fragment = ""
		parsed.RawQuery = ""
		parsed.Path = path.Clean(parsed.Path)
		return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + parsed.EscapedPath()
	}
	return strings.ToLower(strings.TrimSuffix(target, "/"))
}
