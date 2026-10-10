package headless

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type Options struct{ Node, Worker, Pin, Chrome, Profile, ProfileDirectory, BridgePath string }
type Process struct {
	owner         *appshell.OwnedCommand
	input         io.WriteCloser
	operation     chan struct{}
	operationOnce sync.Once
	replies       chan reply
	sequence      atomic.Uint64
}
type reply struct {
	ID     uint64          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type CaptureError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CaptureError) Error() string { return e.Code + ": " + e.Message }

func Validate(options Options) error {
	if options.ProfileDirectory != "" && !appshell.ValidProfileDirectory(options.ProfileDirectory) {
		return errors.New("invalid headless Chrome subprofile")
	}
	for _, path := range []string{options.Node, options.Worker, options.Pin, options.Chrome, options.Profile, options.BridgePath} {
		if !filepath.IsAbs(path) {
			return errors.New("headless runtime paths must be absolute")
		}
	}
	var pin struct {
		Protocol   int    `json:"protocol"`
		NodeSHA256 string `json:"nodeSha256"`
	}
	raw, err := os.ReadFile(options.Pin)
	if err != nil {
		return fmt.Errorf("headless runtime is not packaged: %w", err)
	}
	if err := json.Unmarshal(raw, &pin); err != nil || pin.Protocol != 1 || len(pin.NodeSHA256) != 64 {
		return errors.New("invalid headless runtime provenance")
	}
	binary, err := os.Open(options.Node)
	if err != nil {
		return err
	}
	defer binary.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, binary); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != pin.NodeSHA256 {
		return errors.New("headless Node checksum mismatch")
	}
	for _, path := range []string{options.Worker, options.Chrome, options.BridgePath} {
		if _, err := os.Stat(path); err != nil {
			return err
		}
	}
	return nil
}

func Launch(ctx context.Context, options Options) (*Process, error) {
	if err := Validate(options); err != nil {
		return nil, err
	}
	if options.ProfileDirectory == "" {
		var err error
		options.ProfileDirectory, err = appshell.ResolveProfileDirectory(options.Profile)
		if err != nil {
			return nil, err
		}
	}
	command := exec.Command(options.Node, options.Worker)
	command.Stderr = io.Discard
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	owner, err := appshell.StartOwnedCommand(command)
	if err != nil {
		return nil, err
	}
	p := &Process{owner: owner, input: input, replies: make(chan reply, 1)}
	go func() {
		defer close(p.replies)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 16<<20)
		for scanner.Scan() {
			var value reply
			if json.Unmarshal(scanner.Bytes(), &value) != nil {
				return
			}
			select {
			case p.replies <- value:
			case <-owner.Closed():
				return
			}
		}
	}()
	metadata, err := p.call(ctx, map[string]any{"type": "init", "chrome": options.Chrome, "profile": options.Profile, "profileDirectory": options.ProfileDirectory, "bridgePath": options.BridgePath})
	if err == nil {
		var initialized struct {
			IdleReleaseVersion int `json:"idleReleaseVersion"`
			WorkerDriver       struct {
				ProtocolVersion int `json:"protocolVersion"`
			} `json:"workerDriver"`
		}
		if json.Unmarshal(metadata, &initialized) != nil || initialized.WorkerDriver.ProtocolVersion != 1 {
			err = errors.New("headless worker protocol mismatch")
		} else if initialized.IdleReleaseVersion != 1 {
			err = errors.New("headless worker lacks idle lease support; rebuild the matching packaged worker")
		}
	}
	// Return the owned worker even on failure so cleanup can be verified by its
	// manager. Never lose an uncertain process tree behind a nil result.
	return p, err
}

func (p *Process) call(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	// Waiting for another RPC must respect the caller's deadline too. A lease
	// acknowledgement must not hold admission indefinitely behind a capture.
	p.operationOnce.Do(func() { p.operation = make(chan struct{}, 1) })
	select {
	case p.operation <- struct{}{}:
		defer func() { <-p.operation }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := p.sequence.Add(1)
	request["id"] = id
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	// Once dispatched, do not pretend cancellation stopped Chrome. Await the
	// bounded worker outcome; owning run leases remain held until this returns.
	timeout := 120 * time.Second
	if request["type"] != "capture" {
		if deadline, exists := ctx.Deadline(); exists && time.Until(deadline) < timeout {
			timeout = time.Until(deadline)
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	// The same deadline covers pipe backpressure and the reply. A timed-out
	// write has an uncertain dispatch outcome, so retire the owned tree before
	// releasing the caller's lease or allowing any subsequent request.
	if err := writeWorkerFrame(p.input, append(raw, '\n'), timer.C); err != nil {
		return nil, p.protocolFailure(err.Error())
	}
	select {
	case value, ok := <-p.replies:
		if !ok {
			return nil, p.protocolFailure("headless worker disconnected")
		}
		if value.ID != id {
			return nil, p.protocolFailure("headless response ownership mismatch")
		}
		if !value.OK {
			var failure CaptureError
			if json.Unmarshal(value.Error, &failure) == nil && failure.Code != "" {
				if len(failure.Message) > 600 {
					failure.Message = failure.Message[:600]
				}
				return nil, &failure
			}
			return nil, fmt.Errorf("headless capture failed: %.600s", value.Error)
		}
		return value.Result, nil
	case <-timer.C:
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := p.owner.CloseForRetry(cleanup); err != nil {
			return nil, fmt.Errorf("headless timeout; cleanup unverified: %w", err)
		}
		return nil, errors.New("headless worker timed out and its owned tree was stopped")
	}
}

func writeWorkerFrame(output io.Writer, frame []byte, deadline <-chan time.Time) error {
	written := make(chan error, 1)
	go func() {
		n, err := output.Write(frame)
		if err == nil && n != len(frame) {
			err = io.ErrShortWrite
		}
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			return errors.New("headless worker write failed")
		}
		return nil
	case <-deadline:
		return errors.New("headless worker write timed out")
	}
}
func (p *Process) protocolFailure(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := p.owner.CloseForRetry(ctx); err != nil {
		return fmt.Errorf("%s; cleanup unverified: %w", message, err)
	}
	return errors.New(message)
}
func (p *Process) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	raw, err := p.call(ctx, map[string]any{"type": "capture", "source": source, "payload": payload})
	if err != nil {
		return domain.Observation{}, err
	}
	var observation domain.Observation
	err = json.Unmarshal(raw, &observation)
	return observation, err
}
func (p *Process) Driver() string { return "headless" }

// SetIdleHold preserves source frontiers for the entire collection lease,
// including quiet gaps between capture rounds. It never launches Chrome.
func (p *Process) SetIdleHold(ctx context.Context, held bool) error {
	raw, err := p.call(ctx, map[string]any{"type": "setIdleHold", "held": held})
	if err != nil {
		return err
	}
	var acknowledgement struct {
		Held *bool `json:"held"`
	}
	if json.Unmarshal(raw, &acknowledgement) != nil || acknowledgement.Held == nil || *acknowledgement.Held != held {
		return p.protocolFailure("headless idle hold acknowledgement mismatch")
	}
	return nil
}

func (p *Process) PID() int           { return p.owner.PID() }
func (p *Process) Done() <-chan error { return p.owner.Done() }
func (p *Process) Terminate()         { p.owner.Terminate() }
func (p *Process) CloseForRetry(ctx context.Context) error {
	// The shutdown command closes Chrome normally before owned-tree readback.
	_, _ = p.call(ctx, map[string]any{"type": "shutdown"})
	return p.owner.CloseForRetry(ctx)
}
func (p *Process) ReplacementReadiness(ctx context.Context) error { return ctx.Err() }
func (p *Process) OpenExtensionsPage(context.Context) error {
	return errors.New("open-source or native-post interaction requires browser handoff")
}
