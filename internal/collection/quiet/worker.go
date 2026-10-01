package quiet

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Worker borrows source-scoped hidden targets. Its Job owns Node only; neither
// worker shutdown nor worker failure has authority over the capture Chrome Job.
type Worker struct {
	op       chan struct{}
	writeMu  sync.Mutex
	targets  *Targets
	options  headless.Options
	owner    *appshell.OwnedCommand
	input    io.WriteCloser
	replies  chan workerReply
	sequence uint64
	retired  bool
	failure  error
}

type workerReply struct {
	Type      string          `json:"type"`
	ID        uint64          `json:"id"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result"`
	Error     json.RawMessage `json:"error"`
	RPCID     uint64          `json:"rpcId"`
	Source    domain.Source   `json:"source"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	TimeoutMS int             `json:"timeoutMs"`
}

func NewWorker(targets *Targets, options headless.Options) *Worker {
	return &Worker{targets: targets, options: options, op: make(chan struct{}, 1)}
}

func (w *Worker) start(ctx context.Context) error {
	if w.owner != nil {
		return nil
	}
	if err := headless.Validate(w.options); err != nil {
		return err
	}
	cmd := exec.Command(w.options.Node, w.options.Worker)
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return err
	}
	owner, err := appshell.StartOwnedCommand(cmd)
	if err != nil {
		_ = input.Close()
		_ = output.Close()
		return err
	}
	w.owner, w.input, w.replies = owner, input, make(chan workerReply, 1)
	go w.read(output, owner)
	raw, err := w.call(ctx, map[string]any{"type": "init", "backend": "browser_quiet_hidden", "bridgePath": w.options.BridgePath, "chromeVersion": w.targets.Version()})
	if err != nil {
		return err
	}
	var metadata struct {
		WorkerDriver struct {
			Name            string `json:"name"`
			ProtocolVersion int    `json:"protocolVersion"`
		} `json:"workerDriver"`
	}
	if json.Unmarshal(raw, &metadata) != nil || metadata.WorkerDriver.Name != "aku-quiet-worker" || metadata.WorkerDriver.ProtocolVersion != 1 {
		return errors.New("Quiet worker protocol mismatch")
	}
	return nil
}

func (w *Worker) read(output io.Reader, owner *appshell.OwnedCommand) {
	defer close(w.replies)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var previousRPC uint64
	for scanner.Scan() {
		var reply workerReply
		if json.Unmarshal(scanner.Bytes(), &reply) != nil {
			return
		}
		if reply.Type == "cdp" {
			if reply.RPCID == 0 || reply.RPCID <= previousRPC || reply.TimeoutMS < 1 || reply.TimeoutMS > 15000 {
				return
			}
			previousRPC = reply.RPCID
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(reply.TimeoutMS)*time.Millisecond)
			result, err := w.targets.Call(ctx, reply.Source, reply.Method, reply.Params)
			cancel()
			message := map[string]any{"type": "cdp_result", "rpcId": reply.RPCID, "ok": err == nil}
			if err == nil {
				message["result"] = result
			}
			if w.write(message) != nil {
				return
			}
			continue
		}
		if reply.Type != "" {
			return
		}
		select {
		case w.replies <- reply:
		case <-owner.Closed():
			return
		}
	}
}

func (w *Worker) write(message any) error {
	raw, err := json.Marshal(message)
	if err != nil || len(raw) > 16<<20 {
		return errors.New("Quiet worker frame exceeds limit")
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_, err = w.input.Write(append(raw, '\n'))
	return err
}

// Caller holds op. Once dispatched, keep the run's lease until bounded outcome
// or verified Node/hidden-target cleanup, even if the initiating caller cancels.
func (w *Worker) call(ctx context.Context, message map[string]any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w.sequence++
	message["id"] = w.sequence
	timeout := 120 * time.Second
	if message["type"] != "capture" {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
			timeout = time.Until(deadline)
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	written := make(chan error, 1)
	go func() { written <- w.write(message) }()
	select {
	case err := <-written:
		if err != nil {
			return nil, errors.New("Quiet worker write failed")
		}
	case <-timer.C:
		return nil, errors.New("Quiet worker write timed out")
	}
	select {
	case reply, ok := <-w.replies:
		if !ok || reply.ID != w.sequence {
			return nil, errors.New("Quiet worker disconnected or ownership mismatch")
		}
		if !reply.OK {
			var failure headless.CaptureError
			if json.Unmarshal(reply.Error, &failure) == nil && failure.Code != "" {
				if len(failure.Message) > 600 {
					failure.Message = failure.Message[:600]
				}
				return nil, &failure
			}
			return nil, errors.New("Quiet capture failed")
		}
		return reply.Result, nil
	case <-timer.C:
		return nil, errors.New("Quiet worker timed out")
	}
}

func (w *Worker) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	select {
	case w.op <- struct{}{}:
	case <-ctx.Done():
		return domain.Observation{}, ctx.Err()
	}
	defer func() { <-w.op }()
	if w.retired || w.failure != nil {
		return domain.Observation{}, errors.New("Quiet collector is unavailable; select Adaptive or retry the capture runtime")
	}
	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	startupErr := w.start(startupCtx)
	cancelStartup()
	if startupErr != nil {
		return domain.Observation{}, w.fail(startupErr)
	}
	raw, err := w.call(ctx, map[string]any{"type": "capture", "source": source, "payload": payload})
	if err != nil {
		var sourceFailure *headless.CaptureError
		if errors.As(err, &sourceFailure) && sourceFailure.Code != "capture_timeout" && sourceFailure.Code != "worker_error" && sourceFailure.Code != "response_too_large" {
			return domain.Observation{}, err
		}
		return domain.Observation{}, w.fail(err)
	}
	var observation domain.Observation
	if json.Unmarshal(raw, &observation) != nil {
		return domain.Observation{}, w.fail(errors.New("Quiet observation is invalid"))
	}
	return observation, nil
}

func (w *Worker) fail(cause error) error {
	w.failure = cause
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := w.retire(ctx); err != nil {
		return fmt.Errorf("%w; Quiet cleanup unverified: %v", cause, err)
	}
	return cause
}

func (w *Worker) Retire(ctx context.Context) error {
	select {
	case w.op <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-w.op }()
	return w.retire(ctx)
}

func (w *Worker) retire(ctx context.Context) error {
	w.retired = true
	// No graceful worker command is required: this Job owns Node only. Stop it
	// first so EOF cannot trigger new page calls during hidden-target disposal.
	if w.owner != nil {
		if err := w.owner.CloseForRetry(ctx); err != nil {
			return err
		}
	}
	return w.targets.Close(ctx)
}
