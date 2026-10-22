package httpapi

import (
	"errors"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// calibrationFailureStatus keeps unsupported connector contracts distinct
// from unexpected execution failures, without writing a calibration record.
func calibrationFailureStatus(err error) (int, string) {
	if errors.Is(err, reasoning.ErrCalibrationUnsupported) {
		return http.StatusUnprocessableEntity, "calibration_unsupported"
	}
	var unsupported *reasoning.UnsupportedModelError
	if errors.As(err, &unsupported) {
		return http.StatusUnprocessableEntity, "model_unsupported"
	}
	return http.StatusInternalServerError, "calibration_failed"
}
