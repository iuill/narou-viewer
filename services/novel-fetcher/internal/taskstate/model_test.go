package taskstate

import (
	"errors"
	"strings"
	"testing"
)

var errTestRandomSource = errors.New("random source failure")

func TestNewTaskIDFallsBackWhenRandomSourceFails(t *testing.T) {
	original := taskIDRandomReader
	taskIDRandomReader = func([]byte) (int, error) {
		return 0, errTestRandomSource
	}
	t.Cleanup(func() { taskIDRandomReader = original })

	if id := NewTaskID("test"); !strings.HasPrefix(id, "test-") {
		t.Fatalf("fallback task id = %q", id)
	}
}
