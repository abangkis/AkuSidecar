package engine

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const ExpectedBridgeBuildID = "aku-bridge-0.7.0-preview.1-source-fidelity-v60"

var ErrActionNotFound = errors.New("bridge action not found")
var ErrActionConflict = errors.New("bridge action conflict")

type ReloadAction struct {
	ID                  string  `json:"id"`
	RequestID           string  `json:"requestId"`
	Type                string  `json:"type"`
	Actor               any     `json:"actor"`
	Reason              string  `json:"reason"`
	Status              string  `json:"status"`
	CreatedAt           string  `json:"createdAt"`
	ExpiresAt           string  `json:"expiresAt"`
	DeliveredAt         *string `json:"deliveredAt"`
	AcceptedAt          *string `json:"acceptedAt"`
	HeartbeatObservedAt *string `json:"heartbeatObservedAt"`
	CompletedAt         *string `json:"completedAt"`
	PreviousBuildID     string  `json:"previousBuildId"`
	ExpectedBuildID     string  `json:"expectedBuildId"`
	ObservedBuildID     string  `json:"observedBuildId"`
	ErrorCategory       string  `json:"errorCategory"`
	Message             string  `json:"message"`
	expires             time.Time
}

type ReloadActions struct {
	mu            sync.Mutex
	active        *ReloadAction
	retained      map[string]*ReloadAction
	relayLastSeen time.Time
	timeout       time.Duration
}

func NewReloadActions(timeout time.Duration) *ReloadActions {
	return &ReloadActions{retained: map[string]*ReloadAction{}, timeout: timeout}
}

func (r *ReloadActions) Request(requestID string, actor any, reason, previousBuild string) (ReloadAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire()
	requestID = strings.TrimSpace(requestID)
	reason = strings.TrimSpace(reason)
	if requestID == "" || len(requestID) > 128 {
		return ReloadAction{}, fmt.Errorf("requestId is required and bounded")
	}
	if reason == "" || len(reason) > 500 {
		return ReloadAction{}, fmt.Errorf("reason is required and bounded")
	}
	if replay := r.retained[requestID]; replay != nil {
		if replay.Reason != reason || !reflect.DeepEqual(replay.Actor, actor) {
			return ReloadAction{}, ErrActionConflict
		}
		return *replay, nil
	}
	if r.active != nil && !terminalAction(r.active.Status) {
		return ReloadAction{}, ErrActionConflict
	}
	now := time.Now().UTC()
	action := &ReloadAction{ID: domain.NewID("bridge_action"), RequestID: requestID, Type: "reload_self", Actor: actor, Reason: reason, Status: "pending", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(r.timeout).Format(time.RFC3339Nano), PreviousBuildID: previousBuild, ExpectedBuildID: ExpectedBridgeBuildID, expires: now.Add(r.timeout)}
	r.active = action
	r.retain(action)
	return *action, nil
}
func (r *ReloadActions) Next(wait time.Duration, done <-chan struct{}) (*ReloadAction, error) {
	deadline := time.Now().Add(wait)
	for {
		r.mu.Lock()
		r.relayLastSeen = time.Now()
		r.expire()
		if r.active != nil && r.active.Status == "pending" {
			now := domain.Now()
			r.active.Status = "delivered"
			r.active.DeliveredAt = &now
			r.retain(r.active)
			value := *r.active
			r.mu.Unlock()
			return &value, nil
		}
		r.mu.Unlock()
		if wait <= 0 || time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-done:
			return nil, nil
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func (r *ReloadActions) Accept(id string) (ReloadAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire()
	if r.active == nil || r.active.ID != id {
		return ReloadAction{}, ErrActionNotFound
	}
	if r.active.Status == "accepted" || r.active.Status == "completed" {
		return *r.active, nil
	}
	if r.active.Status != "delivered" {
		return ReloadAction{}, ErrActionConflict
	}
	now := domain.Now()
	r.active.Status = "accepted"
	r.active.AcceptedAt = &now
	r.retain(r.active)
	return *r.active, nil
}
func (r *ReloadActions) Observe(heartbeat domain.BridgeHeartbeat) *ReloadAction {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire()
	if r.active == nil || r.active.Status != "accepted" {
		return nil
	}
	now := domain.Now()
	r.active.ObservedBuildID = heartbeat.BuildID
	r.active.HeartbeatObservedAt = &now
	if heartbeat.BuildID == r.active.ExpectedBuildID {
		r.active.Status = "completed"
		r.active.CompletedAt = &now
		r.active.Message = "AkuBridge reload_self completed and the expected build heartbeat was observed."
	}
	r.retain(r.active)
	value := *r.active
	return &value
}
func (r *ReloadActions) Get(id string) (ReloadAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expire()
	if r.active != nil && r.active.ID == id {
		return *r.active, nil
	}
	for _, value := range r.retained {
		if value.ID == id {
			return *value, nil
		}
	}
	return ReloadAction{}, ErrActionNotFound
}
func (r *ReloadActions) expire() {
	if r.active == nil || terminalAction(r.active.Status) || time.Now().Before(r.active.expires) {
		return
	}
	category := "reload_heartbeat_timeout"
	switch r.active.Status {
	case "pending":
		if r.relayLastSeen.Before(r.active.expires.Add(-r.timeout)) {
			category = "relay_page_stale"
		} else {
			category = "relay_not_delivered"
		}
	case "delivered":
		category = "extension_not_accepted"
	case "accepted":
		if r.active.ObservedBuildID != "" {
			category = "build_mismatch"
		}
	}
	now := domain.Now()
	r.active.Status = "failed"
	r.active.CompletedAt = &now
	r.active.ErrorCategory = category
	r.active.Message = map[string]string{"relay_page_stale": "AkuBrowser relay page did not request the cooperative action before the deadline.", "relay_not_delivered": "AkuBrowser relay did not claim reload_self before the deadline.", "extension_not_accepted": "AkuBridge did not accept the delivered reload_self action before the deadline.", "reload_heartbeat_timeout": "AkuBridge accepted reload_self but no post-reload heartbeat arrived before the deadline.", "build_mismatch": "AkuBridge reloaded but did not announce the expected build identity before the deadline."}[category]
	r.retain(r.active)
}
func (r *ReloadActions) retain(value *ReloadAction) {
	r.retained[value.RequestID] = value
	if len(r.retained) <= 32 {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, item := range r.retained {
		created, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
		if oldestKey == "" || created.Before(oldest) {
			oldestKey = key
			oldest = created
		}
	}
	delete(r.retained, oldestKey)
}
func terminalAction(status string) bool { return status == "completed" || status == "failed" }
