package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestValidateAdminTaskMediaStatus(t *testing.T) {
	if err := validateAdminTaskMediaStatus(model.TaskStatusSucceeded); err != nil {
		t.Fatalf("succeeded task should allow media preview: %v", err)
	}
	for _, status := range []model.TaskStatus{
		model.TaskStatusQueued,
		model.TaskStatusRunning,
		model.TaskStatusFailed,
		model.TaskStatusCancelled,
	} {
		if err := validateAdminTaskMediaStatus(status); err == nil {
			t.Errorf("status %q should not allow media preview", status)
		}
	}
}
