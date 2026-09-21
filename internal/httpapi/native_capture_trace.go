package httpapi

import (
	"context"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/nativetrace"
)

// SetAppShellPID uses the live session owner, including user-requested retries.
// No executable path or process command line crosses the telemetry boundary.
func (s *Server) SetAppShellPID(provider func() int) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	s.appShellPID = provider
}

func (s *Server) startNativeCaptureTrace(command *domain.BridgeCommand) {
	if command.Type != "collect_visible" {
		return
	}
	lease, _ := command.Payload["captureLeaseId"].(string)
	if lease == "" {
		return
	}
	source, _ := command.Payload["source"].(string)
	s.appShellActionsMu.RLock()
	provider := s.appShellPID
	s.appShellActionsMu.RUnlock()
	var pid uint32
	if provider != nil {
		pid = uint32(provider())
	}
	s.nativeTrace.Start(command.RunID, pid, func(sample nativetrace.Sample) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := s.engine.RecordCaptureSurfaceEvent(ctx, domain.CaptureSurfaceEvent{
			ID: domain.NewID("native_trace"), SessionID: lease, RunID: command.RunID, Source: domain.Source(source),
			Event: "native_trace", Outcome: sample.Trigger, OccurredAt: sample.At,
			Detail: map[string]any{"schema": "windows-capture-native-v1", "phase": "command_claimed_capture_cleanup_window", "commandId": command.ID, "browserRootPID": pid, "native": sample},
		})
		if err != nil && s.logger != nil {
			s.logger.Printf("native capture telemetry unavailable")
		}
	})
}
