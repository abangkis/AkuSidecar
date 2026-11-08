package headless

import (
	_ "embed"
	"encoding/json"
	"slices"
)

//go:embed worker/navigation-catalog.json
var navigationCatalogJSON []byte

type navigationDiagnosticCatalog struct {
	ErrorCategories map[string]string `json:"errorCategories"`
	BlockedReasons  []string          `json:"blockedReasons"`
}

var navigationCatalog = func() navigationDiagnosticCatalog {
	var catalog navigationDiagnosticCatalog
	if err := json.Unmarshal(navigationCatalogJSON, &catalog); err != nil {
		panic("invalid Headless navigation diagnostic catalog")
	}
	return catalog
}()

type mainDocumentFailure struct {
	State                 string `json:"state"`
	ErrorCode             string `json:"errorCode"`
	Canceled              *bool  `json:"canceled"`
	BlockedReason         string `json:"blockedReason"`
	HTTPStatus            int    `json:"httpStatus"`
	StatusSource          string `json:"httpStatusSource"`
	RequestMetadataSource string `json:"requestMetadataSource"`
	UserAgentHeadless     *bool  `json:"userAgentHeadless"`
	ClientHintsHeadless   *bool  `json:"clientHintsHeadless"`
}

// NavigationDetails projects only validated fields into persistent run errors.
func (e *CaptureError) NavigationDetails() map[string]any {
	if e.Code != "navigation_failed" || e.CDPCommand != "Page.navigate" {
		return nil
	}
	switch e.ChromeErrorCategory {
	case "aborted", "dns", "connection", "timeout", "tls", "blocked", "other":
	default:
		return nil
	}
	details := map[string]any{"cdpCommand": e.CDPCommand, "chromeErrorCategory": e.ChromeErrorCategory}
	if navigationCatalog.ErrorCategories[e.ChromeErrorCode] == e.ChromeErrorCategory {
		details["chromeErrorCode"] = e.ChromeErrorCode
	}
	var document mainDocumentFailure
	if json.Unmarshal(e.MainDocument, &document) != nil {
		return details
	}
	switch document.State {
	case "matched":
		projected := map[string]any{"state": document.State}
		if _, ok := navigationCatalog.ErrorCategories[document.ErrorCode]; ok {
			projected["errorCode"] = document.ErrorCode
		}
		if document.Canceled != nil {
			projected["canceled"] = *document.Canceled
		}
		if slices.Contains(navigationCatalog.BlockedReasons, document.BlockedReason) {
			projected["blockedReason"] = document.BlockedReason
		}
		switch document.StatusSource {
		case "response_received", "extra_info":
			if document.HTTPStatus >= 200 && document.HTTPStatus <= 599 {
				projected["httpStatus"] = document.HTTPStatus
				projected["httpStatusSource"] = document.StatusSource
			}
		case "not_observed":
			projected["httpStatusSource"] = document.StatusSource
		}
		if document.RequestMetadataSource == "request_extra_info" || document.RequestMetadataSource == "request_will_be_sent" {
			if document.UserAgentHeadless != nil {
				projected["userAgentHeadless"] = *document.UserAgentHeadless
			}
			if document.ClientHintsHeadless != nil {
				projected["clientHintsHeadless"] = *document.ClientHintsHeadless
			}
			if document.UserAgentHeadless != nil || document.ClientHintsHeadless != nil {
				projected["requestMetadataSource"] = document.RequestMetadataSource
			}
		}
		details["mainDocument"] = projected
	case "not_observed", "unavailable":
		details["mainDocument"] = map[string]any{"state": document.State}
	}
	return details
}
