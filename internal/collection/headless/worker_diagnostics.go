package headless

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log"
	"slices"
	"time"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

const (
	maxDiagnosticLine = 1024
	workerEventLimit  = 16
)

//go:embed worker/diagnostic-reasons.json
var reasonCatalog []byte

var workerReasons = func() []string {
	var reasons []string
	if err := json.Unmarshal(reasonCatalog, &reasons); err != nil {
		panic(err) // Invalid embedded build asset.
	}
	return reasons
}()

type workerEvent struct {
	Operation string  `json:"operation"`
	Phase     d.Phase `json:"phase"`
	Level     string  `json:"level"`
	Reason    string  `json:"reason"`
	Sequence  uint64  `json:"sequence"`
}

// WorkerDiagnostics is shared across worker replacements. Each launcher has its own
// queue and output budget, so a noisy worker cannot consume the other launcher's slots.
type WorkerDiagnostics struct {
	headless chan workerEvent
	quiet    chan workerEvent
}

func NewWorkerDiagnostics(ctx context.Context, logger *log.Logger) *WorkerDiagnostics {
	sink := &WorkerDiagnostics{
		headless: make(chan workerEvent, workerEventLimit),
		quiet:    make(chan workerEvent, workerEventLimit),
	}
	go forwardWorkerEvents(ctx, logger, "headless", sink.headless)
	go forwardWorkerEvents(ctx, logger, "quiet", sink.quiet)
	return sink
}

func forwardWorkerEvents(ctx context.Context, logger *log.Logger, launcher string, events <-chan workerEvent) {
	var windowEnd time.Time
	count := 0
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			now := time.Now()
			if !now.Before(windowEnd) {
				count = 0
				windowEnd = now.Add(time.Minute)
			}
			if count < workerEventLimit {
				count++
				operational.Report(logger, "headless_worker", e.Phase, "failed", operational.Fields{
					"launcher": launcher, "sequence": e.Sequence, "reason": e.Reason,
				})
			}
		}
	}
}

func (s *WorkerDiagnostics) HeadlessStderr() io.Writer {
	if s == nil {
		return io.Discard
	}
	return &workerLines{events: s.headless}
}

func (s *WorkerDiagnostics) QuietStderr() io.Writer {
	if s == nil {
		return io.Discard
	}
	return &workerLines{events: s.quiet}
}

// os/exec serializes writes to this per-process stream. Drop an oversized line
// through its delimiter so a valid-looking suffix cannot become a new event.
type workerLines struct {
	events    chan<- workerEvent
	line      []byte
	oversized bool
}

func (w *workerLines) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		part, rest, complete := bytes.Cut(p, []byte{'\n'})
		p = rest
		if len(w.line)+len(part) > maxDiagnosticLine {
			w.oversized = true
		}
		if !w.oversized {
			w.line = append(w.line, part...)
		}
		if complete {
			if !w.oversized {
				w.forwardLine()
			}
			w.line = w.line[:0]
			w.oversized = false
		}
	}
	return n, nil
}

func (w *workerLines) forwardLine() {
	var e workerEvent
	if json.Unmarshal(w.line, &e) != nil || e.Operation != "headless_worker" || e.Level != "error" || e.Sequence == 0 || e.Sequence > 1<<53-1 {
		return
	}
	switch e.Phase {
	case "request_failed", "idle_close", "shutdown_close", "interrupt_close", "fatal":
	default:
		return
	}
	if e.Reason != "worker_error" && !slices.Contains(workerReasons, e.Reason) {
		return
	}
	select {
	case w.events <- e:
	default: // Slow or unavailable diagnostic output must never block RPC or teardown.
	}
}
