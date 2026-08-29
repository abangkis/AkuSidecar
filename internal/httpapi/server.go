package httpapi

import (
	"context"
	"crypto/subtle"
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
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type Server struct {
	config            config.Config
	store             *store.Store
	engine            *engine.Engine
	credentials       credentials.Manager
	http              *http.Server
	listener          net.Listener
	logger            *log.Logger
	started           time.Time
	shutdownRequested chan struct{}
	shutdownOnce      sync.Once
	appShellActionsMu sync.RWMutex
	openExtensions    func(context.Context) error
}

func New(cfg config.Config, state *store.Store, runtime *engine.Engine, logger *log.Logger) (*Server, error) {
	assets, err := fs.Sub(embeddedAssets, "web")
	if err != nil {
		return nil, err
	}
	server := &Server{
		config: cfg, store: state, engine: runtime, logger: logger,
		credentials: credentials.ForRuntime(cfg.Root, cfg.Dev),
		started:     time.Now(), shutdownRequested: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", server.api())
	mux.Handle("/", server.static(http.FS(assets)))
	server.http = &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port), Handler: server.security(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 130 * time.Second, IdleTimeout: 60 * time.Second}
	return server, nil
}

func (s *Server) ShutdownRequested() <-chan struct{} {
	return s.shutdownRequested
}

func (s *Server) requestShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownRequested) })
}

// SetOpenExtensionsAction attaches the currently running app shell without
// making AkuSupervisor or the browser page responsible for browser lifecycle.
func (s *Server) SetOpenExtensionsAction(action func(context.Context) error) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	s.openExtensions = action
}

func (s *Server) openExtensionsPage(ctx context.Context) error {
	s.appShellActionsMu.RLock()
	action := s.openExtensions
	s.appShellActionsMu.RUnlock()
	if action == nil {
		return errors.New("app shell browser action is unavailable")
	}
	return action(ctx)
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
	if err := s.http.Shutdown(ctx); err != nil {
		// Browser polling can keep a request active beyond the bounded drain
		// window. Closing only the remaining HTTP connections is safe after the
		// engine has cancelled all product work and keeps Supervisor shutdown
		// within its five-second grace contract.
		if closeErr := s.http.Close(); closeErr != nil {
			return fmt.Errorf("drain HTTP server: %v; close active connections: %w", err, closeErr)
		}
	}
	return nil
}

func (s *Server) api() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.applyCORS(r, w)
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
	case r.Method == http.MethodPost && p == "/api/app-shell/open-extensions":
		if !s.config.Dev || strings.TrimSpace(s.config.Deployment.Mode) != "development" {
			return notFound("app-shell development action")
		}
		openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := s.openExtensionsPage(openCtx); err != nil {
			return apiError{Status: http.StatusConflict, Code: "app_shell_action_unavailable", Message: "AkuBrowser could not open Chrome Extensions in its development profile.", Details: map[string]any{"reason": err.Error()}}
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
	case r.Method == http.MethodGet && p == "/api/diagnostics/export":
		since := r.URL.Query().Get("since")
		until := r.URL.Query().Get("until")
		sessionID := r.URL.Query().Get("sessionId")
		export, err := s.engine.DiagnosticsExport(ctx, since, until, sessionID)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Disposition", "attachment; filename=aku-diagnostics-"+time.Now().UTC().Format("20060102T150405")+".json")
		return writeJSON(w, http.StatusOK, export)
	case r.Method == http.MethodGet && p == "/api/health":
		settings, err := s.store.GetSettings(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": domain.ApplicationVersion, "runtime": "go", "deployment": s.config.Deployment.PublicStatus(), "provider": s.engine.ProviderName(), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime(), "bridgeContractVersion": domain.BridgeContractVersion, "softwareUpdate": domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion), "instanceEpoch": s.engine.Epoch(), "uptimeMs": time.Since(s.started).Milliseconds(), "database": map[string]any{"status": "healthy"}, "loadProfile": settings.LoadProfile})
	case r.Method == http.MethodGet && p == "/api/runtime/update-readiness":
		ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{
			"ready": ready, "reason": reason, "instanceEpoch": s.engine.Epoch(),
			"controlAvailable": s.config.RuntimeControlToken != "",
		})
	case s.config.Dev && r.Method == http.MethodPost && p == "/api/diagnostics/calibration":
		ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
		if err != nil {
			return err
		}
		if !ready {
			return apiError{Status: http.StatusConflict, Code: "runtime_busy", Message: "Calibration needs an idle runtime.", Details: map[string]any{"reason": reason}}
		}
		var body struct {
			Provider string `json:"provider"`
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				return badRequest("calibration request body must be valid JSON: " + err.Error())
			}
		}
		cfg := s.config
		if providerName := strings.TrimSpace(body.Provider); providerName != "" {
			if err := cfg.Reasoning.Select(providerName); err != nil {
				return apiError{Status: http.StatusBadRequest, Code: "unknown_provider", Message: err.Error()}
			}
		}
		report, calErr := reasoning.RunCalibration(ctx, cfg)
		if report.ModelID == "" && calErr != nil {
			return apiError{Status: http.StatusInternalServerError, Code: "calibration_failed", Message: calErr.Error()}
		}
		if appendErr := appendCalibrationReport(s.calibrationLedgerPath(), report); appendErr != nil {
			s.logger.Printf("calibration ledger append failed: %v", appendErr)
		}
		payload := map[string]any{"report": report}
		if calErr != nil {
			payload["error"] = calErr.Error()
		}
		return writeJSON(w, http.StatusOK, payload)
	case s.config.Dev && r.Method == http.MethodGet && p == "/api/diagnostics/calibration":
		reports, err := readCalibrationReports(s.calibrationLedgerPath(), calibrationLedgerLimit)
		if err != nil {
			return apiError{Status: http.StatusInternalServerError, Code: "calibration_ledger_unreadable", Message: err.Error()}
		}
		return writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
	case r.Method == http.MethodPost && p == "/api/runtime/shutdown-if-idle":
		if !validRuntimeControlToken(s.config.RuntimeControlToken, r.Header.Get("X-Aku-Runtime-Control-Token")) {
			return apiError{Status: http.StatusForbidden, Code: "runtime_control_denied", Message: "Runtime control authorization failed."}
		}
		ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
		if err != nil {
			return err
		}
		if !ready {
			return apiError{Status: http.StatusConflict, Code: "runtime_busy", Message: "Runtime update is blocked by active work.", Details: map[string]any{"reason": reason}}
		}
		if err := writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "instanceEpoch": s.engine.Epoch()}); err != nil {
			return err
		}
		go func() {
			time.Sleep(25 * time.Millisecond)
			s.requestShutdown()
		}()
		return nil
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
		timelineBatches, err := s.engine.TimelineBatchSummaries(ctx)
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
		autoUpdate, err := s.engine.AutoUpdateStatus(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"version": domain.ApplicationVersion, "runtime": "go", "deployment": s.config.Deployment.PublicStatus(), "provider": s.engine.ProviderName(), "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime(), "instanceEpoch": s.engine.Epoch(), "bridgeContractVersion": domain.BridgeContractVersion, "softwareUpdate": domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion), "bridgeToken": token, "bridge": s.engine.BridgeStatus(), "database": map[string]any{"status": "healthy", "schemaVersion": store.SchemaVersion}, "sources": domain.Sources(), "settings": settings, "onboarding": onboarding, "calibration": calibration, "activeSession": sessionProgressProjection(active), "timeline": timeline, "timelineBatches": timelineBatches, "latestCheck": latestCheck, "autoUpdate": autoUpdate})
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
			if errors.Is(err, engine.ErrCalibrationRequiresValidatedCandidate) {
				return apiError{Status: http.StatusConflict, Code: "calibration_sample_unavailable", Message: "The latest check produced no validated calibration entry. Choose Update now again."}
			}
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
		return writeJSON(w, http.StatusOK, map[string]any{"provider": s.engine.ProviderName(), "settings": settings, "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime()})
	case r.Method == http.MethodGet && p == "/api/reasoning/providers/readiness":
		providers, err := s.engine.ReasoningProviderReadiness(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"reasoningProviders": providers})
	case r.Method == http.MethodPut && p == "/api/reasoning/credentials":
		var body struct {
			Provider string `json:"provider"`
			Secret   string `json:"secret"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		providerName := strings.TrimSpace(body.Provider)
		provider, ok := s.config.Reasoning.Providers[providerName]
		if !ok || provider.HideFromSettings {
			return apiError{Status: http.StatusBadRequest, Code: "unknown_provider", Message: "The selected reasoning provider is unavailable."}
		}
		credentialRef := strings.TrimSpace(provider.CredentialRef)
		if credentialRef == "" {
			return apiError{Status: http.StatusBadRequest, Code: "credential_not_supported", Message: "The selected reasoning provider does not use an API credential."}
		}
		if err := s.credentials.Put(credentialRef, body.Secret); err != nil {
			s.logger.Printf("secure credential write failed for ref %q: %v", credentialRef, err)
			return apiError{Status: http.StatusInternalServerError, Code: "credential_store_failed", Message: "AkuBrowser could not save this credential securely."}
		}
		body.Secret = ""
		providerSummaries := s.engine.ReasoningProviders()
		for index := range providerSummaries {
			if providerSummaries[index].Name == providerName {
				providerSummaries[index].Configured = true
				providerSummaries[index].ConfigurationStatus = "ready"
			}
		}
		return writeJSON(w, http.StatusOK, map[string]any{
			"credential":         map[string]any{"provider": providerName, "reference": credentialRef, "configured": true},
			"reasoningProviders": providerSummaries,
		})
	case r.Method == http.MethodPost && p == "/api/reasoning/runtime/discover":
		runtime, err := s.engine.DiscoverReasoningExecutable(ctx)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"reasoningRuntime": runtime})
	case r.Method == http.MethodPut && p == "/api/settings":
		var body struct {
			Settings           domain.Settings `json:"settings"`
			ConfirmationPhrase string          `json:"confirmationPhrase"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		current, err := s.engine.Settings(ctx)
		if err != nil {
			return err
		}
		if body.Settings.AIDetectionEnabled && body.Settings.AIDetectionPresentation == "hide" && current.AIDetectionPresentation != "hide" && body.ConfirmationPhrase != domain.AIHideConfirmationPhrase {
			return badRequest("activating Hide requires the exact confirmation phrase")
		}
		settings, err := s.engine.SaveSettings(ctx, body.Settings)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"provider": s.engine.ProviderName(), "settings": settings, "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime()})
	case r.Method == http.MethodPost && p == "/api/updates":
		var body struct {
			Intent string `json:"intent"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		session, err := s.engine.StartVisibleUpdate(ctx, body.Intent)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"session": session})
	case r.Method == http.MethodGet && p == "/api/sessions/active":
		session, err := s.engine.ActiveSession(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(session)})
	case r.Method == http.MethodGet && p == "/api/inbox":
		limit := boundedInt(r.URL.Query().Get("limit"), 12, 1, 25)
		offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
		sessions, total, err := s.engine.Inbox(ctx, limit, offset)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "total": total, "limit": limit, "offset": offset})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/sessions/") && strings.HasSuffix(p, "/model-usage"):
		sessionID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/sessions/"), "/model-usage")
		if sessionID == "" || strings.Contains(sessionID, "/") {
			return notFound("session")
		}
		usage, err := s.engine.SessionModelUsage(ctx, sessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("session")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
	case r.Method == http.MethodGet && p == "/api/model-usage":
		windowDays := boundedInt(r.URL.Query().Get("windowDays"), 30, 7, 90)
		if windowDays != 7 && windowDays != 30 && windowDays != 90 {
			return badRequest("windowDays must be 7, 30, or 90")
		}
		usage, err := s.engine.AggregateModelUsage(ctx, windowDays)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/trace"):
		runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/trace")
		if runID == "" || strings.Contains(runID, "/") {
			return notFound("run")
		}
		stage := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("stage")))
		if stage == "" {
			stage = "captured"
		}
		if stage != "captured" && stage != "skipped" && stage != "evaluated" && stage != "selected" && stage != "added" {
			return badRequest("stage must be captured, skipped, evaluated, selected, or added")
		}
		limit := boundedInt(r.URL.Query().Get("limit"), 10, 1, 20)
		offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
		trace, err := s.engine.InboxRunTrace(ctx, runID, stage, limit, offset)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("run")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"trace": trace})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/vision-evaluations"):
		runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/vision-evaluations")
		if runID == "" || strings.Contains(runID, "/") {
			return notFound("run")
		}
		jobs, err := s.engine.VisionEvaluationJobs(ctx, runID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("run")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/vision-evaluations/") && strings.HasSuffix(p, "/retry"):
		jobID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/vision-evaluations/"), "/retry")
		if jobID == "" || strings.Contains(jobID, "/") {
			return notFound("vision evaluation")
		}
		if err := s.engine.RetryVisionEvaluation(ctx, jobID); errors.Is(err, sql.ErrNoRows) {
			return notFound("vision evaluation")
		} else if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "jobId": jobID})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/selection-corrections"):
		runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/selection-corrections")
		if runID == "" || strings.Contains(runID, "/") {
			return notFound("run")
		}
		var body struct {
			CandidateRef string `json:"candidateRef"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		if strings.TrimSpace(body.CandidateRef) == "" {
			return badRequest("candidateRef is required")
		}
		correction, item, err := s.engine.CorrectSelection(ctx, runID, body.CandidateRef)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("candidate")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"correction": correction, "item": item})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/re-evaluate"):
		runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/re-evaluate")
		if runID == "" || strings.Contains(runID, "/") {
			return notFound("run")
		}
		run, err := s.engine.ReevaluateFailedRun(ctx, runID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("run")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/selection-corrections/") && strings.HasSuffix(p, "/undo"):
		id := path.Base(strings.TrimSuffix(p, "/undo"))
		correction, err := s.engine.UndoSelectionCorrection(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("selection correction")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"correction": correction})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/sessions/") && !strings.HasSuffix(p, "/cancel"):
		id := path.Base(p)
		session, err := s.engine.Session(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("session")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(&session)})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/sessions/") && strings.HasSuffix(p, "/cancel"):
		id := path.Base(strings.TrimSuffix(p, "/cancel"))
		if err := s.engine.CancelSession(ctx, id); err != nil {
			return err
		}
		session, err := s.engine.Session(ctx, id)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(&session)})
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
		timelineBatches, err := s.engine.TimelineBatchSummaries(ctx)
		if err != nil {
			return err
		}
		autoUpdate, err := s.engine.AutoUpdateStatus(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items, "timelineBatches": timelineBatches, "latestCheck": latestCheck, "autoUpdate": autoUpdate})
	case r.Method == http.MethodGet && p == "/api/library/items":
		query, err := parseLibraryQuery(r.URL.Query())
		if err != nil {
			return err
		}
		result, err := s.engine.Library(ctx, query)
		if err != nil {
			if errors.Is(err, store.ErrMemoryLibraryQuery) {
				return badRequest(err.Error())
			}
			return err
		}
		items := make([]libraryItemView, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, publicLibraryItem(item, false))
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	case r.Method == http.MethodGet && p == "/api/library/saved":
		query, err := parseLibraryQuery(r.URL.Query())
		if err != nil {
			return err
		}
		query.SavedOnly = true
		result, err := s.engine.SavedLibrary(ctx, query)
		if err != nil {
			if errors.Is(err, store.ErrMemoryLibraryQuery) {
				return badRequest(err.Error())
			}
			return err
		}
		items := make([]libraryItemView, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, publicLibraryItem(item, false))
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	case r.Method == http.MethodGet && p == "/api/library/storage":
		limit, err := parseLibraryStorageLimit(r.URL.Query())
		if err != nil {
			return err
		}
		report, err := s.engine.LibraryStorage(ctx, limit)
		if errors.Is(err, store.ErrMemoryStorageRecommendationLimit) {
			return badRequest(err.Error())
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, report)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/release-full-copy"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/release-full-copy")
		if id == "" || strings.Contains(id, "/") {
			return notFound("library item")
		}
		item, err := s.engine.ReleaseLibraryItem(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) || errors.Is(err, store.ErrMemoryTombstoned) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"released": true, "id": id, "retentionTier": item.RetentionTier})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/keep-in-library"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/keep-in-library")
		if id == "" || strings.Contains(id, "/") {
			return notFound("library item")
		}
		item, err := s.engine.KeepMemoryInLibrary(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) || errors.Is(err, store.ErrMemoryNotSaved) {
			return notFound("library item")
		}
		if errors.Is(err, store.ErrSavedMemoryTextUnavailable) {
			return apiError{Status: http.StatusConflict, Code: "saved_memory_text_unavailable", Message: "The saved source text is unavailable, so it cannot be kept as a full copy."}
		}
		if errors.Is(err, store.ErrMemoryTombstoned) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"kept": true, "id": id, "saved": item.Saved, "permanentKeep": item.PermanentKeep, "retentionTier": item.RetentionTier})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/done"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/done")
		if id == "" || strings.Contains(id, "/") {
			return notFound("library item")
		}
		item, err := s.engine.DoneSavedMemory(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if errors.Is(err, store.ErrMemoryTombstoned) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"done": true, "id": id, "saved": item.Saved, "permanentKeep": item.PermanentKeep, "retentionTier": item.RetentionTier})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/forget-permanently"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/forget-permanently")
		if id == "" || strings.Contains(id, "/") {
			return notFound("library item")
		}
		_, err := s.engine.ForgetLibraryItem(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"forgotten": true, "id": id})
	case r.Method == http.MethodDelete && strings.HasPrefix(p, "/api/library/items/"):
		id := strings.TrimPrefix(p, "/api/library/items/")
		if id == "" || strings.Contains(id, "/") {
			return notFound("library item")
		}
		err := s.engine.RemoveLibraryItem(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"removed": true, "id": id})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/library/items/"):
		id := path.Base(p)
		if id == "" || id == "items" {
			return notFound("library item")
		}
		item, err := s.engine.LibraryItem(ctx, id)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"item": publicLibraryItem(item, true)})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/auto-update/batches/") && strings.HasSuffix(p, "/reveal"):
		id := path.Base(strings.TrimSuffix(p, "/reveal"))
		var body struct {
			Presentation string `json:"presentation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		batch, err := s.engine.RevealPreparedBatch(ctx, id, body.Presentation)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"batch": batch})
	case r.Method == http.MethodPost && p == "/api/auto-update/budget/reset":
		status, err := s.engine.ResetAutoUpdateDailyQuota(ctx)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
	case r.Method == http.MethodPost && p == "/api/auto-update/usage-limit/restore":
		status, err := s.engine.ConfirmAutoUpdateUsageRestored(ctx)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
	case r.Method == http.MethodPost && p == "/api/auto-update/prepare":
		session, err := s.engine.StartPreparedUpdateNow(ctx)
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"session": session})
	case r.Method == http.MethodPost && p == "/api/ui/activity":
		s.engine.RecordUIAccess(ctx)
		status, err := s.engine.AutoUpdateStatus(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"recorded": true, "autoUpdate": status})
	case r.Method == http.MethodGet && p == "/api/auto-update/status":
		status, err := s.engine.AutoUpdateStatus(ctx)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/keep-full-copy"):
		// Legacy API compatibility. The embedded UI exposes Read later, while
		// older clients may finish an already-issued Keep request.
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/timeline/"), "/keep-full-copy")
		if id == "" || strings.Contains(id, "/") {
			return notFound("timeline item")
		}
		item, alreadyKept, err := s.engine.KeepTimelineFullCopy(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if errors.Is(err, store.ErrMemoryTombstoned) {
			return apiError{Status: http.StatusConflict, Code: "memory_tombstoned", Message: "This Timeline item was permanently forgotten, so a full copy cannot be kept."}
		}
		if errors.Is(err, store.ErrTimelineMemoryTextUnavailable) {
			return apiError{Status: http.StatusConflict, Code: "timeline_memory_text_unavailable", Message: "The source text is unavailable for this Timeline item, so no full copy was kept."}
		}
		if errors.Is(err, store.ErrTimelineMemoryNotEligible) {
			return apiError{Status: http.StatusConflict, Code: "timeline_memory_not_eligible", Message: "Only a final Timeline item from a completed update can be kept as a full copy."}
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"kept": true, "alreadyKept": alreadyKept, "retentionTier": item.RetentionTier})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/read-later"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/timeline/"), "/read-later")
		if id == "" || strings.Contains(id, "/") {
			return notFound("timeline item")
		}
		item, alreadySaved, err := s.engine.ReadLaterTimeline(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if errors.Is(err, store.ErrMemoryTombstoned) {
			return apiError{Status: http.StatusConflict, Code: "memory_tombstoned", Message: "This Timeline item was permanently forgotten, so it cannot be Saved."}
		}
		if errors.Is(err, store.ErrTimelineMemoryNotEligible) {
			return apiError{Status: http.StatusConflict, Code: "timeline_memory_not_eligible", Message: "Only a final Timeline item from a completed update can be Saved for later."}
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"saved": true, "alreadySaved": alreadySaved, "retentionTier": item.RetentionTier, "permanentKeep": item.PermanentKeep})
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
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/ai-feedback"):
		id := path.Base(strings.TrimSuffix(p, "/ai-feedback"))
		var body domain.AIFeedbackInput
		if err := readJSON(r, &body); err != nil {
			return err
		}
		feedback, err := s.engine.AddAIFeedback(ctx, id, body)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"feedback": feedback})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/ai-feedback"):
		id := path.Base(strings.TrimSuffix(p, "/ai-feedback"))
		feedback, err := s.engine.AIFeedbackHistory(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/ai-feedback/") && strings.HasSuffix(p, "/undo"):
		id := path.Base(strings.TrimSuffix(p, "/undo"))
		feedback, err := s.engine.UndoAIFeedback(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("AI feedback")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/recapture"):
		id := path.Base(strings.TrimSuffix(p, "/recapture"))
		var body struct {
			CaptureMode domain.MediaRecaptureMode   `json:"captureMode"`
			Reason      domain.MediaRecaptureReason `json:"reason"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		if body.CaptureMode != domain.MediaRecaptureBackground && body.CaptureMode != domain.MediaRecaptureForeground {
			return badRequest("captureMode must be background or foreground")
		}
		if body.Reason == "" {
			body.Reason = domain.MediaRecaptureMissingMedia
		}
		if !body.Reason.Valid() {
			return badRequest("reason must be missing_media or playback_error")
		}
		recapture, err := s.engine.QueueMediaRecaptureForReason(ctx, id, body.CaptureMode, body.Reason)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"recapture": recapture})
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
			Relation      string `json:"relation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		correction, err := s.engine.CorrectSemanticEvent(ctx, id, body.Action, body.TargetEventID, body.Relation)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("semantic event report")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"correction": correction})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/timeline/") && strings.HasSuffix(p, "/media-evidence"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(strings.TrimSuffix(p, "/media-evidence"))
		var body domain.PassiveXMediaEvidence
		if err := readJSON(r, &body); err != nil {
			return err
		}
		recapture, updated, err := s.engine.ApplyPassiveXMediaEvidence(ctx, id, r.Header.Get("X-Aku-Bridge-Id"), body)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("timeline item")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		response := map[string]any{"updated": updated}
		if updated {
			response["recapture"] = recapture
		}
		return writeJSON(w, http.StatusOK, response)
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
		if origin := canonicalExtensionOrigin(r.Header.Get("Origin")); origin != "" {
			body.Capabilities.ExtensionOrigin = origin
		} else {
			body.Capabilities.ExtensionOrigin = canonicalExtensionOrigin(body.Capabilities.ExtensionOrigin)
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
	case r.Method == http.MethodGet && p == "/api/bridge/commands/pending":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		runID, err := s.engine.PendingBridgeRunID(ctx)
		if err != nil {
			return err
		}
		if runID == "" {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return writeJSON(w, http.StatusOK, map[string]any{"runId": runID})
	case r.Method == http.MethodPost && p == "/api/bridge/capture-surfaces/events":
		if err := s.requireBridge(r); err != nil {
			return err
		}
		var body struct {
			Events []domain.CaptureSurfaceEvent `json:"events"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		if len(body.Events) < 1 || len(body.Events) > 32 {
			return badRequest("capture surface telemetry must contain between 1 and 32 events")
		}
		recorded := make([]domain.CaptureSurfaceEvent, 0, len(body.Events))
		for _, event := range body.Events {
			value, err := s.engine.RecordCaptureSurfaceEvent(ctx, event)
			if err != nil {
				return badRequest(err.Error())
			}
			recorded = append(recorded, value)
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"events": recorded})
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
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/claim"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(strings.TrimSuffix(p, "/claim"))
		recapture, err := s.engine.ClaimMediaRecapture(ctx, id, r.Header.Get("X-Aku-Bridge-Id"))
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("media recapture")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/observation"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(strings.TrimSuffix(p, "/observation"))
		var body struct {
			Observation domain.Observation `json:"observation"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		recapture, err := s.engine.AcceptMediaRecapture(ctx, id, body.Observation)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/failure"):
		if err := s.requireBridge(r); err != nil {
			return err
		}
		id := path.Base(strings.TrimSuffix(p, "/failure"))
		var body struct {
			Error domain.Failure `json:"error"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		recapture, err := s.engine.FailMediaRecapture(ctx, id, body.Error)
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
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

func validRuntimeControlToken(expected, supplied string) bool {
	if len(expected) < 32 || len(supplied) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
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

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: https://pbs.twimg.com https://video.twimg.com https://licdn.com https://*.licdn.com https://fbcdn.net https://*.fbcdn.net https://fbsbx.com https://*.fbsbx.com https://cdninstagram.com https://*.cdninstagram.com; media-src 'self' https://video.twimg.com https://dms.licdn.com https://fbcdn.net https://*.fbcdn.net https://fbsbx.com https://*.fbsbx.com https://cdninstagram.com https://*.cdninstagram.com; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if !trustedLoopbackHost(r.Host) {
			http.Error(w, "loopback host required", http.StatusForbidden)
			return
		}
		if !s.trustedBrowserOrigin(r) {
			http.Error(w, "origin is not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func trustedLoopbackHost(raw string) bool {
	host := raw
	if parsed, _, err := net.SplitHostPort(raw); err == nil {
		host = parsed
	} else if strings.Contains(raw, ":") {
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "127.0.0.1" || host == "localhost"
}

func (s *Server) trustedBrowserOrigin(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" {
		return true
	}
	origin, err := url.Parse(raw)
	if err != nil || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return false
	}
	if origin.Scheme == "http" && strings.EqualFold(origin.Host, r.Host) {
		return true
	}
	if origin.Scheme != "chrome-extension" || origin.Host == "" || !bridgePath(r.URL.Path) {
		return false
	}
	actual := canonicalExtensionOrigin(raw)
	for _, allowed := range s.config.Bridge.TrustedExtensionOrigins {
		if actual == canonicalExtensionOrigin(allowed) {
			return true
		}
	}
	return false
}

func canonicalExtensionOrigin(raw string) string {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin.Scheme != "chrome-extension" || origin.Host == "" {
		return ""
	}
	return "chrome-extension://" + strings.ToLower(origin.Host)
}

func bridgePath(value string) bool {
	return strings.HasPrefix(value, "/api/bridge/") || strings.HasPrefix(value, "/api/operations/bridge/")
}

func (s *Server) applyCORS(r *http.Request, w http.ResponseWriter) {
	origin := r.Header.Get("Origin")
	if strings.HasPrefix(origin, "chrome-extension://") && bridgePath(r.URL.Path) && s.trustedBrowserOrigin(r) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Aku-Bridge-Token, X-Aku-Bridge-Id, X-Aku-Bridge-Contract")
	}
}

func sessionProgressProjection(session *domain.Session) *domain.Session {
	if session == nil {
		return nil
	}
	projected := *session
	projected.ItemCount = len(session.Items)
	projected.Items = nil
	projected.Coverage = nil
	if stage, ok := session.Coverage["pipelineStage"]; ok {
		projected.Coverage = map[string]any{"pipelineStage": stage}
		if updatedAt, exists := session.Coverage["pipelineStageUpdatedAt"]; exists {
			projected.Coverage["pipelineStageUpdatedAt"] = updatedAt
		}
	}
	projected.Runs = make([]domain.Run, len(session.Runs))
	copy(projected.Runs, session.Runs)
	for index := range projected.Runs {
		projected.Runs[index].Coverage = nil
	}
	return &projected
}

func readJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return apiError{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Content-Type must be application/json"}
	}
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
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
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
