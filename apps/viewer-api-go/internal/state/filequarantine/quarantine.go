package filequarantine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func Move(path string, label string) (string, error) {
	label = strings.Trim(strings.TrimSpace(label), ".- ")
	if label == "" {
		label = "quarantined"
	}
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	base := path + "." + label + "-" + timestamp
	for suffix := 0; ; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", base, suffix+1)
		}
		if _, err := os.Lstat(candidate); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err := os.Rename(path, candidate); err != nil {
			return "", err
		}
		return candidate, nil
	}
}
