package config

import "errors"

const (
	XCaptureExclusive  = "exclusive"
	XCaptureConcurrent = "concurrent"
)

// XCaptureMode is read once at startup. Settings writes cannot change this route.
func (c CaptureConfig) XCaptureMode() string {
	if c.XMode == "" {
		return XCaptureExclusive
	}
	return c.XMode
}

func (c CaptureConfig) ValidateXMode() error {
	switch c.XCaptureMode() {
	case XCaptureExclusive, XCaptureConcurrent:
		return nil
	default:
		return errors.New("capture.xMode must be exclusive or concurrent")
	}
}
