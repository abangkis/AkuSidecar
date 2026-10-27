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
	if c.XReaderSource != "" && c.XReaderSource != "dedicated" && c.XReaderSource != "main" {
		return errors.New("capture.xReaderSource must be dedicated or main")
	}
	if c.XReaderSource == "main" && (c.XCaptureMode() != XCaptureConcurrent || c.XReaderProfile != "") {
		return errors.New("main X reader requires concurrent mode and the ordinary profile, without xReaderProfile")
	}
	switch c.XCaptureMode() {
	case XCaptureExclusive, XCaptureConcurrent:
		return nil
	default:
		return errors.New("capture.xMode must be exclusive or concurrent")
	}
}

func (c CaptureConfig) XUsesMainProfile() bool {
	return c.XCaptureMode() == XCaptureConcurrent && c.XReaderSource == "main"
}
