//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// This receipt stays on the private pipe. Cookie values never enter Go.
type xApplicationSeedReceipt struct {
	AccountID        string `json:"accountId"`
	ReaderPID        uint32 `json:"readerPID"`
	ReaderProfile    string `json:"readerProfile"`
	CollectorPID     uint32 `json:"collectorPID"`
	CollectorProfile string `json:"collectorProfile"`
	Seeded           *bool  `json:"seeded"`
	HomeReady        *bool  `json:"homeReady"`
}

func (xApplicationSeedReceipt) String() string     { return "private-application-seed-receipt" }
func (v xApplicationSeedReceipt) GoString() string { return v.String() }
func (xApplicationSeedReceipt) MarshalJSON() ([]byte, error) {
	return nil, errors.New("private application receipt cannot be serialized")
}

// Only trusted application composition calls this method after explicit manual
// selection. Neither a status flag, package manifest nor generic RPC selects it.
func (b *xApplicationOwner) seedNative(ctx context.Context) (xApplicationSeedReceipt, time.Time, error) {
	var empty xApplicationSeedReceipt
	if b == nil || ctx == nil || ctx.Err() != nil || b.isClosed() {
		return empty, time.Time{}, errors.New("application seed unavailable")
	}
	b.initializationMu.Lock()
	if b.protocolOwner <= 0 || b.seedUsed {
		b.initializationMu.Unlock()
		return empty, time.Time{}, errors.New("application seed unavailable or replayed")
	}
	b.seedUsed = true
	owner := b.protocolOwner
	b.initializationMu.Unlock()
	fail := func(start time.Time, err error) (xApplicationSeedReceipt, time.Time, error) {
		b.terminateOwnedTree()
		if err == nil {
			err = errors.New("application seed receipt rejected")
		}
		return empty, start, err
	}
	// Revalidate OS/profile ownership before acquisition. A process announcement
	// or account receipt is never a substitute for held native process pins.
	b.roleMu.Lock()
	roles := make(map[string]xManualPilotWindowsRole, len(b.roles))
	for _, r := range b.roles {
		roles[r.role] = r
	}
	b.roleMu.Unlock()
	if len(roles) != 2 || roles["reader"].chrome.PID == 0 || roles["collector"].chrome.PID == 0 {
		return fail(time.Time{}, nil)
	}
	backend := &xManualPilotWindowsBackend{roles: roles}
	for _, role := range []string{"reader", "collector"} {
		if _, err := backend.snapshot(ctx, role); err != nil {
			return fail(time.Time{}, errors.New("application seed owner unavailable"))
		}
	}
	started := time.Now()
	call, cancel := context.WithDeadline(ctx, started.Add(nativePilotLife))
	defer cancel()
	reply, err := b.pipe.Call(call, "native_owner_seed", map[string]any{"owner": owner}, nil)
	defer clear(reply.Result)
	if err != nil {
		return fail(started, errors.New("application seed pipe unavailable"))
	}
	if reply.Code != "" {
		if _, ok := nativePilotSeedCode(reply.Code); ok {
			return fail(started, &xApplicationRPCError{code: reply.Code})
		}
		return fail(started, errors.New("application seed rejected"))
	}
	var receipt xApplicationSeedReceipt
	if xManualPilotDecode(reply.Result, &receipt) != nil || !nativePilotAccount(receipt.AccountID) || receipt.Seeded == nil || !*receipt.Seeded || receipt.HomeReady == nil || !*receipt.HomeReady || receipt.ReaderPID != roles["reader"].chrome.PID || receipt.CollectorPID != roles["collector"].chrome.PID || receipt.ReaderProfile != b.cfg.reader.Directory || receipt.CollectorProfile != b.cfg.collector.Directory || call.Err() != nil || !time.Now().Before(started.Add(nativePilotLife)) {
		return fail(started, nil)
	}
	for _, role := range []string{"reader", "collector"} {
		if _, err := backend.snapshot(call, role); err != nil {
			return fail(started, errors.New("application seed owner changed"))
		}
	}
	if call.Err() != nil || !time.Now().Before(started.Add(nativePilotLife)) {
		return fail(started, errors.New("application seed deadline"))
	}
	b.initializationMu.Lock()
	b.seedReady = true
	b.initializationMu.Unlock()
	return receipt, started, nil
}

// The existing closed seed code vocabulary is shared with the retained recipe.
// Do not forward arbitrary peer strings into diagnostics.
func nativePilotSeedCode(code string) (string, bool) {
	catalogs := [][]string{
		{"preconditions", "owner_before", "frame_before", "enrollment_evaluate", "enrollment_project", "enrollment_cleanup", "frame_after", "cookie_read", "cookie_select", "seed_write", "owner_after"},
		{"disabled", "replay", "unavailable", "rejected", "candidate_absent", "bootstrap_missing", "bootstrap_invalid", "wrong_page", "inspection_failed", "document_changed", "ownership_changed", "deadline", "clock_regressed"},
		{"not_attempted", "unknown", "acknowledged"},
	}
	if strings.HasPrefix(code, "manual_seed_v2_") && len(code) <= 80 {
		parts := strings.SplitN(strings.TrimPrefix(code, "manual_seed_v2_"), "_", 4)
		if len(parts) != 4 {
			return "", false
		}
		for i, part := range parts[:3] {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(catalogs[i]) || strconv.Itoa(n) != part {
				return "", false
			}
		}
		initial, ok := nativePilotInitialCode("manual_initial_" + parts[3])
		if !ok || initial.trace == nil {
			return "", false
		}
		return code, true
	}
	for _, phase := range catalogs[0] {
		for _, reason := range catalogs[1] {
			for _, write := range catalogs[2] {
				if code == "manual_seed_"+phase+"_"+reason+"_"+write {
					return code, true
				}
			}
		}
	}
	return "", false
}

var _ json.Marshaler = xApplicationSeedReceipt{}
