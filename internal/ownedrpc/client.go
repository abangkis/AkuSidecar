// Package ownedrpc correlates private pipe calls. It supplies no owner, account
// or operation authority; callers retain framing, allowed-operation and domain checks.
package ownedrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

var (
	ErrClosed  = errors.New("private pipe closed")
	ErrRequest = errors.New("private pipe request invalid")
	ErrWrite   = errors.New("private pipe write failed")
)

// Reply is opaque domain data. Formatting it must not disclose payload contents.
type Reply struct {
	Result     json.RawMessage
	Code       string
	Diagnostic json.RawMessage
}

func (Reply) String() string     { return "private-pipe-reply" }
func (r Reply) GoString() string { return r.String() }

type pendingCall struct {
	reply    chan Reply
	progress func([]byte)
}
type lateObservation struct {
	progress func([]byte)
	until    time.Time
	timer    *time.Timer
}

type Client struct {
	mu      sync.Mutex
	writeMu sync.Mutex
	writer  io.Writer
	next    int64
	pending map[int64]pendingCall
	closed  chan struct{}
	ended   bool
	late    map[int64]lateObservation
}

func New(writer io.Writer) *Client {
	return &Client{writer: writer, pending: map[int64]pendingCall{}, closed: make(chan struct{})}
}

// Call supports concurrent requests with serialized writes. A domain wrapper may
// serialize control operations separately while leaving its capture lane responsive.
func (c *Client) Call(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (Reply, error) {
	return c.call(ctx, op, fields, progress, false, nil)
}

// CallWithLateProgress retains a bounded passive observer after cancellation.
// Late metadata never revives a call or supplies cleanup/operation authority.
func (c *Client) CallWithLateProgress(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (Reply, error) {
	return c.call(ctx, op, fields, progress, true, nil)
}

// CallWithLateProgressPrepared samples caller-owned fields only after the write
// lane is admitted. It is useful for remaining budgets; no domain policy lives here.
func (c *Client) CallWithLateProgressPrepared(ctx context.Context, op string, fields map[string]any, progress func([]byte), prepare func() (map[string]any, error)) (Reply, error) {
	return c.call(ctx, op, fields, progress, true, prepare)
}
func (c *Client) call(ctx context.Context, op string, fields map[string]any, progress func([]byte), retain bool, prepare func() (map[string]any, error)) (Reply, error) {
	if ctx == nil || op == "" || c.writer == nil {
		return Reply{}, ErrRequest
	}
	if err := ctx.Err(); err != nil {
		return Reply{}, err
	}
	if _, ok := fields["id"]; ok {
		return Reply{}, ErrRequest
	}
	if _, ok := fields["op"]; ok {
		return Reply{}, ErrRequest
	}
	c.mu.Lock()
	if c.ended {
		c.mu.Unlock()
		return Reply{}, ErrClosed
	}
	// Exhaustion cannot wrap into a peer-controlled or reused correlation ID.
	if c.next == int64(^uint64(0)>>1) {
		c.mu.Unlock()
		return Reply{}, ErrRequest
	}
	c.next++
	id := c.next
	call := pendingCall{reply: make(chan Reply, 1), progress: progress}
	c.pending[id] = call
	c.mu.Unlock()
	dispatched := false
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		_, unresolved := c.pending[id]
		delete(c.pending, id)
		for key, value := range c.late {
			if time.Now().After(value.until) {
				delete(c.late, key)
			}
		}
		if retain && dispatched && progress != nil && unresolved && !c.ended && ctx.Err() != nil && len(c.late) < 32 {
			if c.late == nil {
				c.late = map[int64]lateObservation{}
			}
			timer := time.AfterFunc(30*time.Second, func() { c.mu.Lock(); delete(c.late, id); c.mu.Unlock() })
			c.late[id] = lateObservation{progress, time.Now().Add(30 * time.Second), timer}
		}
	}()
	message := map[string]any{"id": id, "op": op}
	for key, value := range fields {
		message[key] = value
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return Reply{}, ErrRequest
	}
	defer func() { clear(encoded) }()
	c.writeMu.Lock()
	// Calls canceled while waiting for the writer must not start a late operation.
	if err = ctx.Err(); err == nil {
		c.mu.Lock()
		ended := c.ended
		c.mu.Unlock()
		if ended {
			err = ErrClosed
		} else {
			if prepare != nil {
				var updated map[string]any
				updated, err = prepare()
				if err == nil {
					if _, exists := updated["id"]; exists {
						err = ErrRequest
					}
					if _, exists := updated["op"]; exists {
						err = ErrRequest
					}
					if err == nil {
						message = map[string]any{"id": id, "op": op}
						for key, value := range updated {
							message[key] = value
						}
						clear(encoded)
						encoded, err = json.Marshal(message)
						if err != nil {
							err = ErrRequest
						}
					}
				}
			}
			if err == nil {
				encoded = append(encoded, '\n')
				var n int
				n, err = c.writer.Write(encoded)
				if err == nil && n != len(encoded) {
					err = io.ErrShortWrite
				}
				if err != nil {
					err = ErrWrite
					// Retire the corrupt stream before a queued writer can enter.
					c.Close()
				} else {
					dispatched = true
				}
			}
		}
	}
	c.writeMu.Unlock()
	if err != nil {
		return Reply{}, err
	}
	select {
	case reply := <-call.reply:
		if err := ctx.Err(); err != nil {
			return Reply{}, err
		}
		return reply, nil
	case <-ctx.Done():
		return Reply{}, ctx.Err()
	case <-c.closed:
		// Deliver retires the correlation before EOF. Preserve its terminal
		// reply even when Close wins scheduling; the domain still validates it.
		select {
		case reply := <-call.reply:
			if err := ctx.Err(); err != nil {
				return Reply{}, err
			}
			return reply, nil
		default:
			return Reply{}, ErrClosed
		}
	}
}

// Deliver ignores unknown, duplicate and retired correlations. Its return is
// local correlation accounting only. Framing/domain validation stays with callers.
func (c *Client) Deliver(id int64, reply Reply) bool {
	if len(reply.Diagnostic) > 2048 {
		reply.Diagnostic = nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	call, ok := c.pending[id]
	if !ok {
		if late, exists := c.late[id]; exists {
			late.timer.Stop()
			delete(c.late, id)
		}
	}
	if ok {
		// Publish the buffered terminal reply before retiring its correlation.
		// Close must not observe retirement before the reply becomes readable.
		call.reply <- reply
		delete(c.pending, id)
	}
	return ok
}

func (c *Client) Progress(id int64, raw []byte) {
	if len(raw) > 512 {
		return
	}
	c.mu.Lock()
	call, ok := c.pending[id]
	if !ok {
		if late, found := c.late[id]; found {
			if time.Now().Before(late.until) {
				call.progress = late.progress
				ok = true
			} else {
				delete(c.late, id)
			}
		}
	}
	c.mu.Unlock()
	if ok && call.progress != nil {
		call.progress(raw)
	}
}

// Close ends admission and unresolved correlations. A write already admitted by
// the write-lane state check may be in progress. Its process owner must close
// the actual pipe/Job to interrupt I/O; this method is not an external stop proof.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ended {
		c.ended = true
		clear(c.pending)
		for _, late := range c.late {
			late.timer.Stop()
		}
		clear(c.late)
		close(c.closed)
	}
}

// Pending is a local bookkeeping count, never proof of transport or session health.
func (c *Client) Pending() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.pending) }
