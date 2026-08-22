package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
