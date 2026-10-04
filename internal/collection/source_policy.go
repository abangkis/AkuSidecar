package collection

import (
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// PreferredDriver plans normal collection before process ownership/admission.
// Facebook's experimental headless capability remains available to explicit
// QA, but it is not the normal collector for the approved hybrid direction.
func PreferredDriver(mode string, source domain.Source) (string, error) {
	if mode != "browser" && mode != "headless" {
		return "", errors.New("unsupported collection mode")
	}
	if !HeadlessSourceSupported(source) {
		return "", errors.New("unsupported collection source")
	}
	if mode == "browser" || source == domain.SourceFacebook {
		return "browser", nil
	}
	return "headless", nil
}
