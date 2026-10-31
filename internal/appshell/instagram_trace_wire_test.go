package appshell

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInstagramRealNodeWire(t *testing.T) {
	for _, scenario := range []string{"missing", "throw", "scope", "eof", "lost", "read_command", "read_reply", "read_empty", "read_cleanup", "document_handoff", "trace_saturation", "trace_saturation_eof", "elapsed_unknown"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command("node", "instagram_trace_wire_fixture.mjs", scenario)
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { in.Close(); cmd.Wait() }()
			nonce := strings.Repeat("a", 48)
			bootstrap, _ := json.Marshal(map[string]any{"op": "main_transport_bootstrap", "schema": "aku.main-browser-transport.v1", "nonce": nonce, "transport": "inherited-stdio"})
			in.Write(append(bootstrap, '\n'))
			scan := bufio.NewScanner(out)
			if !scan.Scan() || !strings.Contains(scan.Text(), `"ready":true`) {
				t.Fatal("synthetic bootstrap failed")
			}
			rpc := ownedrpc.New(in)
			go func() {
				defer rpc.Close()
				for scan.Scan() {
					var frame struct {
						ID     int64           `json:"id"`
						Event  string          `json:"event"`
						OK     bool            `json:"ok"`
						Result json.RawMessage `json:"result"`
						Error  struct {
							Code       string          `json:"code"`
							Diagnostic json.RawMessage `json:"diagnostic"`
						} `json:"error"`
					}
					if json.Unmarshal(scan.Bytes(), &frame) != nil {
						continue
					}
					if frame.Event != "" {
						rpc.Progress(frame.ID, frame.Result)
					} else {
						rpc.Deliver(frame.ID, ownedrpc.Reply{Result: frame.Result, Code: frame.Error.Code, Diagnostic: frame.Error.Diagnostic})
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			ctx, a := igdiagnostic.Ensure(ctx, "corroboration")
			var progressMu sync.Mutex
			var progressFrames []igdiagnostic.Wire
			call := func(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (json.RawMessage, error) {
				reply, e := rpc.CallWithLateProgress(ctx, op, fields, func(raw []byte) {
					if scenario == "elapsed_unknown" {
						var frame igdiagnostic.Wire
						if json.Unmarshal(raw, &frame) == nil {
							progressMu.Lock()
							progressFrames = append(progressFrames, frame)
							progressMu.Unlock()
						}
					}
					progress(raw)
				})
				if e != nil {
					return nil, e
				}
				if reply.Code != "" {
					return nil, &MainBrowserError{Code: reply.Code, Diagnostic: reply.Diagnostic}
				}
				return reply.Result, nil
			}
			_, err = instagramTraceRequest(ctx, "main_instagram_bootstrap_corroboration", nonce, call, ValidateInstagramBootstrapCorroborationDiagnostic)
			raw := a.SnapshotJSON()
			if _, ok := igdiagnostic.ValidateJSON(raw); !ok {
				t.Fatalf("invalid cross-language snapshot: %s", raw)
			}
			if strings.Contains(string(raw), "PRIVATE_MARKER") {
				t.Fatal("private text escaped")
			}
			var e igdiagnostic.Envelope
			json.Unmarshal(raw, &e)
			if scenario == "elapsed_unknown" {
				progressMu.Lock()
				frames := append([]igdiagnostic.Wire(nil), progressFrames...)
				progressMu.Unlock()
				var seenKnown bool
				for _, frame := range frames {
					if frame.ElapsedMS != nil {
						seenKnown = true
					}
				}
				if err != nil || !seenKnown || len(frames) == 0 {
					t.Fatalf("expected valid and then unknown elapsed progress: %s", raw)
				}
				last := frames[len(frames)-1]
				if last.ElapsedMS != nil || e.NodeElapsedMS != nil || e.NodeSequence != last.Sequence || e.Complete || e.Cleanup.State != "verified" {
					t.Fatalf("unknown Node elapsed or independent cleanup lost across wire: %s", raw)
				}
			}
			if scenario == "missing" && (err == nil || e.Disposition != "not_started" || e.Cause.Condition != "factory_missing") {
				t.Fatalf("early guard lost: %s", raw)
			}
			if scenario == "throw" && (err == nil || e.Disposition != "started" || e.Cause.Condition != "unclassified_exception") {
				t.Fatalf("exception not closed: %s", raw)
			}
			if scenario == "eof" && (err == nil || e.Disposition != "started" || e.Cleanup.State != "pending" || e.Cause.Condition != "transport_closed") {
				t.Fatalf("EOF invented completion: %s", raw)
			}
			if scenario == "lost" && (err == nil || e.Disposition != "result_lost" || e.Cleanup.State != "verified") {
				t.Fatalf("lost result conflated with execution: %s", raw)
			}
			if scenario == "scope" && (err != nil || e.Disposition != "completed" || e.Cleanup.State != "uncertain") {
				t.Fatalf("scope fault lost: %s", raw)
			}
			if scenario == "document_handoff" && (err != nil || e.Cause == nil || e.Cause.Stage != "handoff" || e.Cause.Condition != "document_within_navigation" || e.Cleanup.State != "verified" || e.Evidence["documentBound"] != "invalidated") {
				t.Fatalf("document handoff observation lost across wire: %s", raw)
			}
			if scenario == "trace_saturation" && (err != nil || e.Complete || e.NodeSequence != 10000 || e.Disposition != "completed" || e.Cause == nil || e.Cause.Condition != "root_request_duplicate" || e.Cleanup.State != "verified") {
				t.Fatalf("saturated terminal trace lost incomplete state, cause or cleanup: %s", raw)
			}
			if scenario == "trace_saturation_eof" && (err == nil || e.Complete || e.NodeSequence != 9999 || e.Disposition != "started" || e.Cause == nil || e.Cause.Condition != "root_request_duplicate" || e.Cleanup.State != "pending") {
				t.Fatalf("saturated missing terminal invented completion or lost cause: %s", raw)
			}
			if expected, ok := map[string]string{"read_command": "read_command_failed", "read_reply": "read_reply_invalid", "read_empty": "read_empty_nonfinal", "read_cleanup": "read_command_failed"}[scenario]; ok {
				cleanup := "verified"
				if scenario == "read_cleanup" {
					cleanup = "uncertain"
				}
				if err != nil || e.Disposition != "completed" || e.Cause == nil || e.Cause.Stage != "body_read" || e.Cause.Condition != expected || e.Cleanup.State != cleanup {
					t.Fatalf("stream failure classification lost across wire: %s", raw)
				}
				if scenario == "read_cleanup" && e.Cleanup.Condition != "stream_close_failed" {
					t.Fatal("cleanup replaced read cause")
				}
			}
		})
	}
}
