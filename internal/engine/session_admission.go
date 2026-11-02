package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) StartVisibleUpdate(ctx context.Context, intent string) (domain.Session, error) {
	trigger := domain.UpdateTriggerUser
	firstRunStatus, err := e.store.CalibrationFirstRunStatus(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if firstRunStatus == "pending" {
		trigger = domain.UpdateTriggerOnboarding
	}
	return e.startSession(ctx, intent, domain.UpdatePolicy{
		Trigger: trigger, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
}

func (e *Engine) grantedActiveSources(settings domain.Settings) []domain.Source {
	if e.headlessEffective() {
		e.mu.RLock()
		allowed := append([]domain.Source(nil), e.headlessAccess...)
		e.mu.RUnlock()
		valid := []domain.Source{}
		for _, source := range settings.ActiveSources {
			for _, granted := range allowed {
				if source == granted {
					valid = append(valid, source)
					break
				}
			}
		}
		return valid
	}
	status := e.BridgeStatus()
	if !status.Compatible || status.Actual == nil {
		return nil
	}
	ready := make(map[string]bool, len(status.Actual.SourceAccess.Sources))
	for _, source := range status.Actual.SourceAccess.Sources {
		ready[source.Source] = source.Ready
	}
	valid := make([]domain.Source, 0, len(settings.ActiveSources))
	for _, source := range settings.ActiveSources {
		if ready[string(source)] {
			valid = append(valid, source)
		}
	}
	return valid
}

func noGrantedActiveSourceError() error {
	return errors.New("No active source is ready in AkuBridge. Open AkuBrowser Settings, use each source's Grant access or Sign in action, accept Chrome's permission prompt, and let its capture script register.")
}

func (e *Engine) startSession(ctx context.Context, intent string, policy domain.UpdatePolicy) (domain.Session, error) {
	e.operation.Lock()
	operationLocked := true
	defer func() {
		if operationLocked {
			e.operation.Unlock()
		}
	}()
	if e.collectionRuntime != nil && e.collectionRuntime.Status().Pending {
		return domain.Session{}, errors.New("collection mode transition is pending; finish active capture or close source/reader windows")
	}
	if err := e.store.RequireHealthyDatabase(ctx); err != nil {
		return domain.Session{}, err
	}
	if err := policy.Validate(); err != nil {
		return domain.Session{}, err
	}
	onboarding, err := e.store.Onboarding(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if onboarding.Status != "completed" {
		return domain.Session{}, errors.New("complete onboarding before starting an update")
	}
	if recapturing, err := e.store.ActiveMediaRecapture(ctx); err != nil {
		return domain.Session{}, err
	} else if recapturing {
		return domain.Session{}, errors.New("finish the active media recapture before starting an update")
	}
	calibration, err := e.store.ActiveCalibration(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if calibration != nil {
		return domain.Session{}, errors.New("complete the active calibration before starting another update")
	}
	status := e.BridgeStatus()
	if !e.headlessEffective() && !status.Compatible {
		return domain.Session{}, fmt.Errorf("AkuBridge v2 is not ready: %s", strings.Join(status.Reasons, "; "))
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if settings.CollectionMode == "headless" && (e.collectionRuntime == nil || e.captureOwner == nil) {
		return domain.Session{}, errors.New("hybrid collection requires the managed capture runtime")
	}
	if e.collectionRuntime != nil && e.collectionRuntime.Status().Requested != settings.CollectionMode {
		return domain.Session{}, errors.New("collection runtime does not match the saved collection mode")
	}
	validSources := e.grantedActiveSources(settings)
	{
		for _, source := range settings.ActiveSources {
			if e.dailySourceReady(source) {
				found := false
				for _, v := range validSources {
					if v == source {
						found = true
					}
				}
				if !found {
					validSources = append(validSources, source)
				}
			}
		}
	}
	if err := e.dailyCleanupPending(); err != nil {
		return domain.Session{}, err
	}
	validSources, err = e.dailyCaptureSources(validSources)
	if err != nil {
		return domain.Session{}, err
	}
	validSources, err = e.captureBehaviorSources(ctx, validSources)
	if err != nil {
		return domain.Session{}, err
	}
	if len(validSources) == 0 {
		return domain.Session{}, noGrantedActiveSourceError()
	}
	settings.ActiveSources = validSources
	if err := e.validateDevelopmentSelection(settings); err != nil {
		return domain.Session{}, err
	}
	e.cancelDeepDetections()
	var lease *captureruntime.Lease
	if settings.CollectionMode != "headless" {
		lease, err = e.acquireCaptureLease()
		if err != nil {
			return domain.Session{}, err
		}
	}
	var session domain.Session
	if sources := e.readyDailySources(); len(sources) > 0 {
		session, err = e.store.CreateDailySourceUpdateSession(ctx, intent, settings, policy, sources)
	} else {
		session, err = e.store.CreateUpdateSession(ctx, intent, settings, policy)
	}
	if err != nil {
		lease.Release()
		return domain.Session{}, err
	}
	e.rememberCaptureSession(session.ID, lease)
	// The queued session is durable now. Do not hold operation while acquiring
	// schedule: finalization can hold schedule while acquiring operation for
	// calibration, including when the preceding daily batch just completed.
	e.operation.Unlock()
	operationLocked = false
	if _, err := e.startNext(ctx, session.ID); err != nil {
		return domain.Session{}, err
	}
	return e.store.GetSession(ctx, session.ID)
}
