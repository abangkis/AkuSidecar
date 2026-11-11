package httpapi

import (
	"context"
	"crypto/subtle"
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
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/nativetrace"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type Server struct {
	config              config.Config
	store               *store.Store
	engine              *engine.Engine
	credentials         credentials.Manager
	http                *http.Server
	listener            net.Listener
	logger              *log.Logger
	started             time.Time
	shutdownRequested   chan struct{}
	shutdownOnce        sync.Once
	appShellActionsMu   sync.RWMutex
	openExtensions      func(context.Context) error
	nativeReaderClose   func(context.Context) error
	nativeReaderGate    chan struct{}
	nativeReaderGateMu  sync.Mutex
	nativeReaderClosing bool
	nativeReaderEpoch   uint64
	nativeReaderWaiters int
	appShellStartup     *appshell.Startup
	nativeTrace         nativetrace.Manager
	appShellPID         func() int
	splitCapture        *splitCaptureTransport

	instagramReaderDiagnostic                 func(context.Context, string) (json.RawMessage, error)
	instagramBootstrapDiagnostic              func(context.Context, string) (json.RawMessage, error)
	instagramBootstrapCorroborationDiagnostic func(context.Context, string) (json.RawMessage, error)

	// Set only by the tagged frontend QA composition, before Server.Start.
	developmentNativeReaderClosed bool
	uiPerformanceMu               sync.RWMutex
	uiPerformanceLatest           *uiPerformanceSnapshot
	postScreenshotGate            chan struct{}
	postScreenshotRenderer        func(context.Context, domain.TimelineItem, string) (postScreenshotResult, error)
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
		nativeReaderGate:   make(chan struct{}, 1),
		postScreenshotGate: make(chan struct{}, 1),
	}
	if (cfg.WindowsCaptureSplit || cfg.Dev && runtime.XConcurrentConfigured()) && goruntime.GOOS == "windows" {
		server.splitCapture = newSplitCaptureTransport()
		runtime.SetBrowserCollectionCleanup(server.ReleaseBrowserCollectionSurfaces)
		runtime.SetBrowserMediaRecaptureDispatch(server.DispatchBrowserMediaRecapture)
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

func (s *Server) SetNativeReaderCloseAction(action func(context.Context) error) {
	s.appShellActionsMu.Lock()
	s.nativeReaderClose = action
	s.appShellActionsMu.Unlock()
}

func (s *Server) nativeReaderCloseAction() func(context.Context) error {
	s.appShellActionsMu.RLock()
	defer s.appShellActionsMu.RUnlock()
	return s.nativeReaderClose
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
	if t := s.splitCapture; t != nil {
		if s.listener == nil {
			t.close()
		} else {
			// A telemetry writer can hold the transport mutex during SQLite I/O.
			// Keep cleanup ownership until close finishes, but do not let that
			// wait prevent the existing HTTP deadline fallback from cancelling it.
			closed := make(chan struct{})
			go func() { defer close(closed); t.close() }()
			select {
			case <-closed:
			case <-ctx.Done():
				s.nativeTrace.Stop()
				if closeErr := s.http.Close(); closeErr != nil {
					return fmt.Errorf("drain HTTP server: %v; close active connections: %w", ctx.Err(), closeErr)
				}
				return nil
			}
		}
	}
	s.nativeTrace.Stop()
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
	if strings.HasPrefix(p, "/api/split-capture/") || strings.HasPrefix(p, "/api/bridge/split-capture/") {
		return s.routeSplitCapture(w, r, p)
	}
	if p == "" {
		p = "/"
	}
	switch {
	case r.Method == http.MethodPost && p == "/api/diagnostics/instagram-reader":
		return s.runInstagramReaderDiagnostic(w, r)
	case r.Method == http.MethodPost && p == "/api/diagnostics/instagram-bootstrap":
		return s.runInstagramReaderDiagnostic(w, r)
	case r.Method == http.MethodGet && p == "/api/diagnostics/instagram/status":
		return s.instagramDiagnosticStatus(w, r)
	case r.Method == http.MethodPost && p == "/api/diagnostics/instagram-bootstrap-corroboration":
		return s.runInstagramReaderDiagnostic(w, r)
	case r.Method == http.MethodPost && p == "/api/post-screenshots":
		return s.createPostScreenshot(w, r)
	case r.Method == http.MethodPost && p == "/api/app-shell/startup-ready":
		return s.acknowledgeAppShellStartup(w, r)
	case r.Method == http.MethodPost && p == "/api/app-shell/open-extensions":
		return s.openExtensionsHTTP(w, r)
	case r.Method == http.MethodGet && p == "/api/diagnostics/export":
		return s.exportDiagnostics(w, r)
	case p == "/api/diagnostics/ui-performance" && (r.Method == http.MethodGet || r.Method == http.MethodPost):
		return s.uiPerformance(w, r)
	case r.Method == http.MethodGet && p == "/api/health":
		return s.health(w, r)
	case r.Method == http.MethodGet && p == "/api/collection/runtime":
		return s.collectionRuntimeStatus(w, r)
	case r.Method == http.MethodPost && p == "/api/development/instagram-trial/start":
		return s.instagramTrialAction(w, r, ctx, "start")
	case r.Method == http.MethodPost && p == "/api/development/instagram-trial/retry":
		return s.instagramTrialAction(w, r, ctx, "retry")
	case r.Method == http.MethodPost && p == "/api/development/instagram-trial/stop":
		return s.instagramTrialAction(w, r, ctx, "stop")
	case r.Method == http.MethodPost && p == "/api/development/x-daily/start":
		return s.startXDaily(w, r)
	case r.Method == http.MethodPost && p == "/api/development/x-daily/stop":
		return s.stopXDaily(w, r)
	case r.Method == http.MethodPost && p == "/api/development/x-manual/start":
		return s.startXManual(w, r)
	case r.Method == http.MethodPost && p == "/api/development/x-manual/stop":
		return s.stopXManual(w, r)
	case r.Method == http.MethodPost && p == "/api/collection/interactive":
		return s.prepareInteractiveCapture(w, r)
	case r.Method == http.MethodGet && p == "/api/runtime/update-readiness":
		return s.runtimeUpdateReadiness(w, r)
	case s.config.Dev && r.Method == http.MethodPost && p == "/api/diagnostics/calibration":
		return s.runCalibrationDiagnostic(w, r)
	case s.config.Dev && r.Method == http.MethodGet && p == "/api/diagnostics/calibration":
		return s.calibrationDiagnosticReports(w, r)
	case r.Method == http.MethodPost && p == "/api/runtime/shutdown-if-idle":
		return s.shutdownIfIdle(w, r)
	case r.Method == http.MethodGet && p == "/api/database/health":
		return s.databaseHealth(w, r)
	case r.Method == http.MethodGet && p == "/api/timeline/storage":
		return s.timelineStorage(w, r)
	case r.Method == http.MethodPost && p == "/api/database/cleanup":
		return s.cleanDatabase(w, r)
	case r.Method == http.MethodGet && p == "/api/bootstrap":
		return s.bootstrap(w, r)
	case r.Method == http.MethodGet && p == "/api/calibration/active":
		return s.activeCalibration(w, r)
	case r.Method == http.MethodPost && p == "/api/calibration/sessions":
		return s.startCalibration(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/calibration/sessions/") && !strings.Contains(strings.TrimPrefix(p, "/api/calibration/sessions/"), "/samples/"):
		return s.calibrationSession(w, r, p)
	case r.Method == http.MethodPut && strings.HasPrefix(p, "/api/calibration/sessions/"):
		return s.decideCalibrationSample(w, r, p)
	case r.Method == http.MethodGet && p == "/api/onboarding":
		return s.onboarding(w, r)
	case r.Method == http.MethodPut && p == "/api/onboarding":
		return s.completeOnboarding(w, r)
	case r.Method == http.MethodGet && p == "/api/settings":
		return s.settings(w, r)
	case r.Method == http.MethodGet && p == "/api/reasoning/providers/readiness":
		return s.reasoningProviderReadiness(w, r)
	case r.Method == http.MethodPut && p == "/api/reasoning/credentials":
		return s.saveReasoningCredential(w, r)
	case r.Method == http.MethodPost && p == "/api/reasoning/runtime/discover":
		return s.discoverReasoningRuntime(w, r)
	case r.Method == http.MethodPut && p == "/api/settings":
		return s.saveSettings(w, r)
	case r.Method == http.MethodPost && p == "/api/updates":
		return s.startUpdate(w, r)
	case r.Method == http.MethodGet && p == "/api/sessions/active":
		return s.activeSession(w, r)
	case r.Method == http.MethodGet && p == "/api/inbox":
		return s.inboxSessions(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/sessions/") && strings.HasSuffix(p, "/model-usage"):
		return s.sessionModelUsage(w, r, p)
	case r.Method == http.MethodGet && p == "/api/model-usage":
		return s.aggregateModelUsage(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/trace"):
		return s.inboxRunTrace(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/vision-evaluations"):
		return s.visionEvaluationJobs(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/vision-evaluations/") && strings.HasSuffix(p, "/retry"):
		return s.retryVisionEvaluation(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/selection-corrections"):
		return s.correctSelection(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/inbox/runs/") && strings.HasSuffix(p, "/re-evaluate"):
		return s.reevaluateFailedRun(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/selection-corrections/") && strings.HasSuffix(p, "/undo"):
		return s.undoSelectionCorrection(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/sessions/") && !strings.HasSuffix(p, "/cancel"):
		return s.sessionDetail(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/sessions/") && strings.HasSuffix(p, "/cancel"):
		return s.cancelSession(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/runs/"):
		return s.runDetail(w, r, p)
	case r.Method == http.MethodGet && p == "/api/timeline":
		return s.timeline(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/content-context"):
		return s.contentContext(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/content-context-feedback"):
		return s.addContentContextFeedback(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/content-context-feedback/") && strings.HasSuffix(p, "/undo"):
		return s.undoContentContextFeedback(w, r, p)
	case r.Method == http.MethodGet && p == "/api/living-topics":
		return s.livingTopics(w, r)
	case r.Method == http.MethodGet && p == "/api/living-topics/notifications":
		return s.livingTopicNotifications(w, r)
	case r.Method == http.MethodPost && p == "/api/living-topics":
		return s.createLivingTopic(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/living-topic-moves/") && strings.HasSuffix(p, "/undo"):
		return s.undoLivingTopicMove(w, r, p)
	case strings.HasPrefix(p, "/api/living-topics/"):
		return s.livingTopicRoute(w, r, p)
	case r.Method == http.MethodGet && p == "/api/library/items":
		return s.libraryItems(w, r)
	case r.Method == http.MethodGet && p == "/api/library/saved":
		return s.savedLibraryItems(w, r)
	case r.Method == http.MethodGet && p == "/api/library/storage":
		return s.libraryStorage(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/release-full-copy"):
		return s.releaseLibraryFullCopy(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/keep-in-library"):
		return s.keepLibraryItem(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/done"):
		return s.doneSavedMemory(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/library/items/") && strings.HasSuffix(p, "/forget-permanently"):
		return s.forgetLibraryItem(w, r, p)
	case r.Method == http.MethodDelete && strings.HasPrefix(p, "/api/library/items/"):
		return s.removeLibraryItem(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/library/items/"):
		return s.libraryItem(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/auto-update/batches/") && strings.HasSuffix(p, "/reveal"):
		return s.revealPreparedBatch(w, r, p)
	case r.Method == http.MethodPost && p == "/api/auto-update/budget/reset":
		return s.resetAutoUpdateBudget(w, r)
	case r.Method == http.MethodPost && p == "/api/capture/x/confirm-recovery":
		return s.confirmXCaptureRecovery(w, r)
	case r.Method == http.MethodPost && p == "/api/auto-update/usage-limit/restore":
		return s.restoreAutoUpdateUsage(w, r)
	case r.Method == http.MethodPost && p == "/api/auto-update/prepare":
		return s.prepareAutoUpdate(w, r)
	case r.Method == http.MethodPost && p == "/api/collection/native-reader/close-and-resume":
		return s.closeNativeReaderHTTP(w, r)
	case r.Method == http.MethodPost && p == "/api/ui/activity":
		return s.recordUIActivity(w, r)
	case r.Method == http.MethodGet && p == "/api/auto-update/status":
		return s.autoUpdateStatus(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/keep-full-copy"):
		return s.keepTimelineFullCopy(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/read-later"):
		return s.readLaterTimeline(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/feedback"):
		return s.addTimelineFeedback(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/ai-feedback"):
		return s.addAIFeedback(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/ai-feedback"):
		return s.aiFeedbackHistory(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/ai-feedback/") && strings.HasSuffix(p, "/undo"):
		return s.undoAIFeedback(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/media-recaptures/"):
		return s.mediaRecaptureStatus(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/recapture"):
		return s.queueMediaRecapture(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/event-suggestions"):
		return s.semanticEventSuggestions(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/timeline/") && strings.HasSuffix(p, "/event-correction"):
		return s.correctSemanticEvent(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/timeline/") && strings.HasSuffix(p, "/media-evidence"):
		return s.acceptPassiveMediaEvidence(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/event-corrections/") && strings.HasSuffix(p, "/undo"):
		return s.undoSemanticCorrection(w, r, p)
	case r.Method == http.MethodPost && p == "/api/bridge/heartbeat":
		return s.bridgeHeartbeat(w, r)
	case r.Method == http.MethodPost && p == "/api/operations/bridge/actions/reload-self":
		return s.requestBridgeReload(w, r)
	case r.Method == http.MethodGet && p == "/api/operations/bridge/actions/next":
		return s.nextBridgeAction(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/operations/bridge/actions/") && strings.HasSuffix(p, "/accept"):
		return s.acceptBridgeAction(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/operations/bridge/actions/"):
		return s.bridgeActionStatus(w, r, p)
	case r.Method == http.MethodGet && p == "/api/bridge/health":
		return s.bridgeHealth(w, r)
	case r.Method == http.MethodGet && p == "/api/bridge/commands/next":
		return s.nextBridgeCommand(w, r)
	case r.Method == http.MethodGet && p == "/api/bridge/commands/pending":
		return s.pendingBridgeCommand(w, r)
	case r.Method == http.MethodPost && p == "/api/bridge/capture-surfaces/events":
		return s.captureSurfaceEvents(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/commands/") && strings.HasSuffix(p, "/observation"):
		return s.acceptBridgeCommandObservation(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/commands/") && strings.HasSuffix(p, "/failure"):
		return s.failBridgeCommand(w, r, p)
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/claim"):
		return s.claimBridgeMediaRecapture(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/observation"):
		return s.acceptBridgeMediaRecaptureObservation(w, r, p)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/api/bridge/media-recaptures/") && strings.HasSuffix(p, "/failure"):
		return s.failBridgeMediaRecapture(w, r, p)
	case r.Method == http.MethodPost && p == "/api/operations/reset-learning":
		return s.resetLearning(w, r)
	case r.Method == http.MethodPost && p == "/api/operations/full-reset":
		return s.fullReset(w, r)
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
	if s.splitCapture != nil && splitCaptureOwnedRoute(r) && !s.splitCapture.authorized(r) {
		return apiError{Status: http.StatusConflict, Code: "capture_instance_mismatch", Message: "This operation belongs to the active Windows capture process."}
	}
	return nil
}

func (s *Server) static(files http.FileSystem) http.Handler {
	handler := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.serveSplitCaptureAsset(w, r) {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.writeError(w, notFound("route"))
			return
		}
		extension := path.Ext(r.URL.Path)
		if extension == ".mjs" {
			// Windows file associations can report text/plain for ES modules.
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		} else if value := mime.TypeByExtension(extension); value != "" {
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: https://pbs.twimg.com https://video.twimg.com https://licdn.com https://*.licdn.com https://fbcdn.net https://*.fbcdn.net https://fbsbx.com https://*.fbsbx.com https://cdninstagram.com https://*.cdninstagram.com; media-src 'self' https://video.twimg.com https://dms.licdn.com https://fbcdn.net https://*.fbcdn.net https://fbsbx.com https://*.fbsbx.com https://cdninstagram.com https://*.cdninstagram.com blob:; connect-src 'self' https://video.twimg.com/amplify_video/ https://video.twimg.com/ext_tw_video/ https://video.twimg.com/tweet_video/; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
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
		headers := "Content-Type, X-Aku-Bridge-Token, X-Aku-Bridge-Id, X-Aku-Bridge-Contract"
		if s.splitCapture != nil {
			headers += ", X-Aku-Capture-Instance"
		}
		w.Header().Set("Access-Control-Allow-Headers", headers)
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
