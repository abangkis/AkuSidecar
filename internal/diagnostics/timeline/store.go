package timeline

import (
	"time"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/snapshot"
)

// Store retains the latest UI performance report. Its zero value is ready to
// use and must not be copied after first use.
type Store struct {
	latest snapshot.Store[Snapshot]
}

// Save records the already validated report with a server-generated UTC time.
func (s *Store) Save(report Report) {
	s.latest.Save(Snapshot{
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Report:     report,
	})
}

// Latest returns an independent snapshot, or nil before the first report.
func (s *Store) Latest() *Snapshot {
	value, ok := s.latest.Latest()
	if !ok {
		return nil
	}
	return &value
}

// Clone returns an independent snapshot, including every optional event value.
func (s Snapshot) Clone() Snapshot {
	s.Report = cloneUIPerformanceReport(s.Report)
	return s
}
