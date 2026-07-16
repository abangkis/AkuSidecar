package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type Server struct {
	config   config.Config
	store    *store.Store
	engine   *engine.Engine
	http     *http.Server
	listener net.Listener
	logger   *log.Logger
	started  time.Time
}

func New(cfg config.Config, state *store.Store, runtime *engine.Engine, logger *log.Logger) (*Server, error) {
	assets, err := fs.Sub(embeddedAssets, "web")
	if err != nil {
		return nil, err
	}
	server := &Server{config: cfg, store: state, engine: runtime, logger: logger, started: time.Now()}
	mux := http.NewServeMux()
	mux.Handle("/api/", server.api())
	mux.Handle("/", server.static(http.FS(assets)))
	server.http = &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port), Handler: security(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 130 * time.Second, IdleTimeout: 60 * time.Second}
	return server, nil
}

func (s *Server) Start() (net.Addr, error) {
	listener, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return nil, err
	}
	s.listener = listener
	go func() {
		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("HTTP server failed: %v", err)
		}
	}()
	return listener.Addr(), nil
}
func (s *Server) Stop(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) api() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyCORS(r, w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("X-Aku-Sidecar-Instance-Epoch", s.engine.Epoch())
		if err := s.route(w, r); err != nil {
			s.writeError(w, err)
		}
	})
}

type apiError struct {
	Status        int
	Code, Message string
	Details       any
}

func (e apiError) Error() string { return e.Message }

func (s *Server) route(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := strings.TrimSuffix(r.URL.Path, "/")
	if p == "" {
		p = "/"
	}
	switch {
	case r.Method == http.MethodGet && p == "/api/health":
		settings, err := s.store.GetSettings(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": domain.ApplicationVersion, "runtime": "go", "provider": s.engine.ProviderName(), "bridgeContractVersion": domain.BridgeContractVersion, "instanceEpoch": s.engine.Epoch(), "uptimeMs": time.Since(s.started).Milliseconds(), "database": map[string]any{"status": "healthy"}, "loadProfile": settings.LoadProfile})
	case r.Method == http.MethodGet && p == "/api/bootstrap":
		settings, err := s.store.GetSettings(ctx)
		if err != nil {
			return err
		}
		token, err := s.store.BridgeToken(ctx)
		if err != nil {
			return err
		}
		active, err := s.engine.ActiveSession(ctx)
		if err != nil {
			return err
		}
		timeline, err := s.engine.Timeline(ctx, settings.TimelineCapacity, 0)
		if err != nil {
			return err
		}
		latestCheck, err := s.engine.LatestTimelineCheck(ctx)
		if err != nil {
			return err
		}
		onboarding, err := s.engine.Onboarding(ctx)
		if err != nil {
			return err
		}
		calibration, err := s.engine.CalibrationOverview(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"version": domain.ApplicationVersion, "runtime": "go", "provider": s.engine.ProviderName(), "instanceEpoch": s.engine.Epoch(), "bridgeContractVersion": domain.BridgeContractVersion, "bridgeToken": token, "bridge": s.engine.BridgeStatus(), "database": map[string]any{"status": "healthy", "schemaVersion": 2}, "settings": settings, "onboarding": onboarding, "calibration": calibration, "activeSession": active, "timeline": timeline, "latestCheck": latestCheck})
	case r.Method == http.MethodGet && p == "/api/calibration/active":
		calibration, err := s.engine.CalibrationOverview(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration.Active})
	case r.Method == http.MethodPost && p == "/api/calibration/sessions":
		var body struct {
			UnifiedSessionID string `json:"unifiedSessionId"`
			TriggerKind      string `json:"triggerKind"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		calibration, err := s.engine.StartCalibration(ctx, body.UnifiedSessionID, body.TriggerKind)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"calibration": calibration})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/calibration/sessions/") && !strings.Contains(strings.TrimPrefix(p, "/api/calibration/sessions/"), "/samples/"):
		id := path.Base(p)
		calibration, err := s.engine.Calibration(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("calibration")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration})
	case r.Method == http.MethodPut && strings.HasPrefix(p, "/api/calibration/sessions/"):
		parts := strings.Split(strings.TrimPrefix(p, "/api/calibration/sessions/"), "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] != "samples" {
			return notFound("calibration route")
		}
		ordinal, err := strconv.Atoi(parts[2])
		if err != nil || ordinal < 0 || ordinal > 9 {
			return badRequest("calibration sample ordinal must be between 0 and 9")
		}
		var decision domain.CalibrationDecision
		if err := readJSON(r, &decision); err != nil {
			return err
		}
		calibration, err := s.engine.DecideCalibration(ctx, parts[0], ordinal, decision)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration})
	case r.Method == http.MethodGet && p == "/api/onboarding":
		onboarding, err := s.engine.Onboarding(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"onboarding": onboarding})
	case r.Method == http.MethodPut && p == "/api/onboarding":
		var body struct {
			ActiveSources []domain.Source `json:"activeSources"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		onboarding, err := s.engine.CompleteOnboarding(ctx, body.ActiveSources)
		if err != nil {
			return badRequest(err.Error())
		}
		settings, err := s.engine.Settings(ctx)
		if err != nil {
			return err
		}
		calibration, err := s.engine.CalibrationOverview(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"onboarding": onboarding, "settings": settings, "calibration": calibration})
	case r.Method == http.MethodGet && p == "/api/settings":
		settings, err := s.engine.Settings(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
	case r.Method == http.MethodPut && p == "/api/settings":
		var body struct {
			Settings domain.Settings `json:"settings"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		settings, err := s.engine.SaveSettings(ctx, body.Settings)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
	case r.Method == http.MethodPost && p == "/api/sessions":
		var body struct {
			Intent string `json:"intent"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		session, err := s.engine.StartSession(ctx, body.Intent)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"session": session})
	case r.Method == http.MethodGet && p == "/api/sessions/active":
		session, err := s.engine.ActiveSession(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": session})
	case r.Method == http.MethodGet && p == "/api/inbox":
		limit := boundedInt(r.URL.Query().Get("limit"), 12, 1, 25)
		offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
		sessions, total, err := s.engine.Inbox(ctx, limit, offset)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "total": total, "limit": limit, "offset": offset})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/sessions/") && !strings.HasSuffix(p, "/cancel"):
		id := path.Base(p)
		session, err := s.engine.Session(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("session")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": session})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/sessions/") && strings.HasSuffix(p, "/cancel"):
		id := path.Base(strings.TrimSuffix(p, "/cancel"))
		if err := s.engine.CancelSession(ctx, id); err != nil {
			return err
		}
		session, err := s.engine.Session(ctx, id)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": session})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/runs/"):
		id := path.Base(p)
		run, err := s.engine.Run(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("run")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"run": run})
	case r.Method == http.MethodGet && p == "/api/timeline":
		limit := boundedInt(r.URL.Query().Get("limit"), 24, 1, 50)
		offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
		items, err := s.engine.Timeline(ctx, limit, offset)
		if err != nil {
			return err
		}
		latestCheck, err := s.engine.LatestTimelineCheck(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items, "latestCheck": latestCheck})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/feedback"):
		id := path.Base(strings.TrimSuffix(p, "/feedback"))
		var value domain.Feedback
		if err := readJSON(r, &value); err != nil {
			return err
		}
		feedback, err := s.engine.AddFeedback(ctx, id, value)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"feedback": feedback})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/event-suggestions"):
		id := path.Base(strings.TrimSuffix(p, "/event-suggestions"))
		limit := boundedInt(r.URL.Query().Get("limit"), 3, 1, 3)
		suggestions, err := s.engine.SemanticEventSuggestions(ctx, id, limit)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/event-correction"):
		id := path.Base(strings.TrimSuffix(p, "/event-correction"))
		var body struct {
			Action        string `json:"action"`
			TargetEventID string `json:"targetEventId"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		correction, err := s.engine.CorrectSemanticEvent(ctx, id, body.Action, body.TargetEventID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("semantic event report")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"correction": correction})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/event-corrections/") && strings.HasSuffix(p, "/undo"):
		id := path.Base(strings.TrimSuffix(p, "/undo"))
		correction, err := s.engine.UndoSemanticCorrection(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("event correction")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"correction": correction})
	case r.Method == http.MethodPost && p == "/api/bridge/heartbeat":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		var body struct {
			Capabilities domain.BridgeHeartbeat `json:"capabilities"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"instanceEpoch": s.engine.Epoch(), "bridge": s.engine.RecordHeartbeat(body.Capabilities)})
	case r.Method == http.MethodPost && p == "/api/operations/bridge/actions/reload-self":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		var body struct {
			RequestID string `json:"requestId"`
			Actor     any    `json:"actor"`
			Reason    string `json:"reason"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		action, err := s.engine.RequestBridgeReload(body.RequestID, body.Actor, body.Reason)
		if errors.Is(err, engine.ErrActionConflict) {
			return conflict(err.Error())
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"action": action})
	case r.Method == http.MethodGet && p == "/api/operations/bridge/actions/next":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		wait := boundedInt(r.URL.Query().Get("waitMs"), 0, 0, 30000)
		action, err := s.engine.NextBridgeAction(time.Duration(wait)*time.Millisecond, r.Context().Done())
		if err != nil {
			return err
		}
		if action == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return writeJSON(w, http.StatusOK, map[string]any{"action": action})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/operations/bridge/actions/") && strings.HasSuffix(p, "/accept"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(strings.TrimSuffix(p, "/accept"))
		action, err := s.engine.AcceptBridgeAction(id)
		if errors.Is(err, engine.ErrActionNotFound) {
			return notFound("bridge action")
		}
		if errors.Is(err, engine.ErrActionConflict) {
			return conflict(err.Error())
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"action": action})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/operations/bridge/actions/"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(p)
		action, err := s.engine.BridgeAction(id)
		if errors.Is(err, engine.ErrActionNotFound) {
			return notFound("bridge action")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"action": action})
	case r.Method == http.MethodGet && p == "/api/bridge/health":
		return writeJSON(w, http.StatusOK, map[string]any{"bridge": s.engine.BridgeStatus()})
	case r.Method == http.MethodGet && p == "/api/bridge/commands/next":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		runID := r.URL.Query().Get("runId")
		if runID == "" {
			return badRequest("runId is required")
		}
		bridgeID := r.Header.Get("X-Aku-Bridge-Id")
		command, err := s.engine.ClaimCommand(ctx, runID, bridgeID)
		if err != nil {
			return err
		}
		if command == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return writeJSON(w, http.StatusOK, map[string]any{"command": command})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/commands/") && strings.HasSuffix(p, "/observation"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		commandID := path.Base(strings.TrimSuffix(p, "/observation"))
		var body struct {
			RunID       string             `json:"runId"`
			Observation domain.Observation `json:"observation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		run, err := s.engine.AcceptObservation(ctx, commandID, body.RunID, body.Observation)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/commands/") && strings.HasSuffix(p, "/failure"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		commandID := path.Base(strings.TrimSuffix(p, "/failure"))
		var body struct {
			RunID string         `json:"runId"`
			Error domain.Failure `json:"error"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		run, err := s.engine.FailCommand(ctx, commandID, body.RunID, body.Error)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"run": run})
	case r.Method == http.MethodPost && p == "/api/operations/reset-learning":
		var body struct {
			Confirmation string `json:"confirmation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		if body.Confirmation != "RESET LEARNING" {
			return badRequest("learning reset requires the exact confirmation RESET LEARNING")
		}
		if err := s.engine.ResetLearning(ctx); err != nil {
			if strings.Contains(err.Error(), "update is running") {
				return conflict(err.Error())
			}
			return err
		}
		calibration, err := s.engine.CalibrationOverview(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"status": "reset", "operation": "reset_learning", "calibration": calibration})
	case r.Method == http.MethodPost && p == "/api/operations/full-reset":
		var body struct {
			Confirmation string `json:"confirmation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		if body.Confirmation != "RESET AKUBROWSER" {
			return badRequest("full reset requires the exact confirmation RESET AKUBROWSER")
		}
		reset, err := s.engine.FullReset(ctx)
		if err != nil {
			if strings.Contains(err.Error(), "update is running") {
				return conflict(err.Error())
			}
			return err
		}
		onboarding, err := s.engine.Onboarding(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"status": "reset", "operation": "full_reset", "reset": reset, "onboarding": onboarding})
	default:
		return notFound("route")
	}
}

func (s *Server) requireBridge(r *http.Request) error {
	if r.Header.Get("X-Aku-Bridge-Contract") != domain.BridgeContractVersion {
		return apiError{Status: http.StatusUnauthorized, Code: "invalid_bridge_contract", Message: "unsupported Bridge contract"}
	}
	if !s.store.MatchesBridgeToken(r.Context(), r.Header.Get("X-Aku-Bridge-Token")) {
		return apiError{Status: http.StatusUnauthorized, Code: "invalid_bridge_token", Message: "invalid Bridge token"}
	}
	return nil
}

func (s *Server) static(files http.FileSystem) http.Handler {
	handler := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.writeError(w, notFound("route"))
			return
		}
		extension := path.Ext(r.URL.Path)
		if value := mime.TypeByExtension(extension); value != "" {
			w.Header().Set("Content-Type", value)
		}
		w.Header().Set("Cache-Control", "no-cache")
		handler.ServeHTTP(w, r)
	})
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: https://pbs.twimg.com https://video.twimg.com https://licdn.com https://*.licdn.com; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
func applyCORS(r *http.Request, w http.ResponseWriter) {
	origin := r.Header.Get("Origin")
	if strings.HasPrefix(origin, "chrome-extension://") {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Aku-Bridge-Token, X-Aku-Bridge-Id, X-Aku-Bridge-Contract")
	}
}

func readJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1_000_001))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return badRequest("request body must be valid JSON: " + err.Error())
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(raw)
	return err
}
func (s *Server) writeError(w http.ResponseWriter, err error) {
	value := apiError{Status: http.StatusInternalServerError, Code: "internal_error", Message: "internal server error"}
	if errors.As(err, &value) {
	} else {
		s.logger.Printf("request failed: %v", err)
	}
	_ = writeJSON(w, value.Status, map[string]any{"error": value.Code, "message": value.Message, "details": value.Details})
}
func badRequest(message string) apiError {
	return apiError{Status: http.StatusBadRequest, Code: "invalid_request", Message: message}
}
func conflict(message string) apiError {
	return apiError{Status: http.StatusConflict, Code: "conflict", Message: message}
}
func notFound(kind string) apiError {
	return apiError{Status: http.StatusNotFound, Code: "not_found", Message: kind + " not found"}
}
func boundedInt(raw string, fallback, min, max int) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
