package httpapi

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/timeline"
)

const uiPerformanceMaxBodyBytes = 64 * 1024

func uiPerformanceDiagnosticsEnabled(cfg config.Config) bool {
	return cfg.Dev && strings.TrimSpace(cfg.Deployment.Mode) == "development"
}

func readUIPerformanceReport(w http.ResponseWriter, r *http.Request) (timeline.Report, error) {
	var report timeline.Report
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return report, apiError{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Content-Type must be application/json"}
	}
	limitedBody := http.MaxBytesReader(w, r.Body, uiPerformanceMaxBodyBytes)
	raw, err := io.ReadAll(limitedBody)
	_ = limitedBody.Close()
	if err != nil {
		return report, badRequest("invalid UI performance report")
	}
	report, err = timeline.DecodeReport(raw)
	if err != nil {
		return report, badRequest("invalid UI performance report")
	}
	return report, nil
}

func (s *Server) uiPerformance(w http.ResponseWriter, r *http.Request) error {
	if !uiPerformanceDiagnosticsEnabled(s.config) {
		return notFound("UI performance diagnostics")
	}
	if r.Method == http.MethodGet {
		return writeJSON(w, http.StatusOK, map[string]any{"trace": s.uiPerformanceReports.Latest()})
	}
	report, err := readUIPerformanceReport(w, r)
	if err != nil {
		return err
	}
	s.uiPerformanceReports.Save(report)
	return writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}
