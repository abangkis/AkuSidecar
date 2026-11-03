package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/ai4u-inference-sdk-go/calibration"
)

const calibrationLedgerLimit = 20

func (s *Server) calibrationLedgerPath() string {
	return filepath.Join(s.config.Root, "runtime", "calibration-results.jsonl")
}

func appendCalibrationReport(path string, report calibration.Report) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode calibration report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("prepare calibration ledger directory: %w", err)
	}
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open calibration ledger: %w", err)
	}
	defer handle.Close()
	if _, err := handle.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append calibration report: %w", err)
	}
	return nil
}

func readCalibrationReports(path string, limit int) ([]calibration.Report, error) {
	handle, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []calibration.Report{}, nil
		}
		return nil, fmt.Errorf("open calibration ledger: %w", err)
	}
	defer handle.Close()
	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read calibration ledger: %w", err)
	}
	reports := make([]calibration.Report, 0, len(lines))
	for index := len(lines) - 1; index >= 0; index-- {
		var report calibration.Report
		if err := json.Unmarshal([]byte(lines[index]), &report); err != nil {
			continue
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (s *Server) runCalibrationDiagnostic(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return apiError{Status: http.StatusConflict, Code: "runtime_busy", Message: "Calibration needs an idle runtime.", Details: map[string]any{"reason": reason}}
	}
	var body struct {
		Provider string `json:"provider"`
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			return badRequest("calibration request body must be valid JSON: " + err.Error())
		}
	}
	cfg := s.config
	if providerName := strings.TrimSpace(body.Provider); providerName != "" {
		if err := cfg.Reasoning.Select(providerName); err != nil {
			return apiError{Status: http.StatusBadRequest, Code: "unknown_provider", Message: err.Error()}
		}
	}
	report, calErr := reasoning.RunCalibration(ctx, cfg)
	if report.ModelID == "" && calErr != nil {
		status, code := calibrationFailureStatus(calErr)
		return apiError{Status: status, Code: code, Message: calErr.Error()}
	}
	if appendErr := appendCalibrationReport(s.calibrationLedgerPath(), report); appendErr != nil {
		s.logger.Printf("calibration ledger append failed: %v", appendErr)
	}
	payload := map[string]any{"report": report}
	if calErr != nil {
		payload["error"] = calErr.Error()
	}
	return writeJSON(w, http.StatusOK, payload)
}

func (s *Server) calibrationDiagnosticReports(w http.ResponseWriter, r *http.Request) error {
	reports, err := readCalibrationReports(s.calibrationLedgerPath(), calibrationLedgerLimit)
	if err != nil {
		return apiError{Status: http.StatusInternalServerError, Code: "calibration_ledger_unreadable", Message: err.Error()}
	}
	return writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}
