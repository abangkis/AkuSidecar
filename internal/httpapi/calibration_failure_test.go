package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestCalibrationFailureStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unsupported calibration", fmt.Errorf("wrapped: %w", reasoning.ErrCalibrationUnsupported), http.StatusUnprocessableEntity, "calibration_unsupported"},
		{"unsupported model", fmt.Errorf("wrapped: %w", &reasoning.UnsupportedModelError{Provider: "ollama", ModelID: "retired-model"}), http.StatusUnprocessableEntity, "model_unsupported"},
		{"execution failure", errors.New("execution failed"), http.StatusInternalServerError, "calibration_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, code := calibrationFailureStatus(tc.err)
			if status != tc.status || code != tc.code {
				t.Fatalf("got %d/%s, want %d/%s", status, code, tc.status, tc.code)
			}
		})
	}
}
