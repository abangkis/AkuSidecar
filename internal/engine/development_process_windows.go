//go:build windows && auth_session_fixture && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DevelopmentFrontendNativeScenarioNormal                = "normal"
	DevelopmentFrontendNativeScenarioForegroundOnly        = "foreground_only"
	DevelopmentFrontendNativeScenarioReaderOpenParentDeath = "reader_open_parent_death"
)

const developmentFrontendNativeScenarioEnv = "AKU_DEV_FRONTEND_NATIVE_SCENARIO"

func developmentFrontendNativeScenario() (string, error) {
	scenario := os.Getenv(developmentFrontendNativeScenarioEnv)
	if scenario == "" {
		return DevelopmentFrontendNativeScenarioNormal, nil
	}
	if err := validateDevelopmentFrontendNativeScenario(scenario); err != nil {
		return "", err
	}
	return scenario, nil
}

func validateDevelopmentFrontendNativeScenario(scenario string) error {
	if scenario != DevelopmentFrontendNativeScenarioNormal && scenario != DevelopmentFrontendNativeScenarioForegroundOnly && scenario != DevelopmentFrontendNativeScenarioReaderOpenParentDeath {
		return errors.New("native frontend scenario rejected")
	}
	return nil
}

func DevelopmentFrontendNativeScenario() (string, error) {
	return developmentFrontendNativeScenario()
}

func validateDevelopmentFrontendNativeStart(start processFixtureStart, scenario string) error {
	if err := validateDevelopmentFrontendNativeScenario(scenario); err != nil {
		return err
	}
	if start.NativeHost == nil || start.Grant.Reopen || os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED") != "" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" || start.NativeHost.Pipe != os.Getenv("AKU_BROKER_FIXTURE_PIPE") {
		return errors.New("native frontend scoped startup frame rejected")
	}
	if err := readerbroker.ValidateFixturePipe(start.NativeHost.Pipe); err != nil {
		return err
	}
	if err := start.NativeHost.ValidateApplied(filepath.Dir(start.Grant.Root)); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil || !samePath(self, start.NativeHost.MainPath) {
		return errors.New("native frontend main receipt identity rejected")
	}
	return nil
}

// RunDevelopmentSessionFixture is a concrete synthetic provider composition,
// dispatched by the actual main binary before ordinary startup. The private
// controller grant still validates parent creation/image and scoped fresh profile.
func RunDevelopmentSessionFixture(input io.Reader, output io.Writer) error {
	return runAuthSessionProcessFixture(input, output, true, nil)
}

func prepareDevelopmentProcessCheckpoint(ctx context.Context, e *Engine, storeScopeID, profileID string, freshEnrollment bool) error {
	capability := DevelopmentSessionCapability{
		Provider: "synthetic-fixture", StoreScopeID: storeScopeID, ProfileID: profileID,
		ExpectedAccounts: map[domain.Source]string{domain.SourceX: "alpha", domain.SourceInstagram: "alpha"},
		Sources:          []domain.Source{domain.SourceX, domain.SourceInstagram},
	}
	return e.PrepareDevelopmentSessionCheckpoint(ctx, capability, freshEnrollment)
}

func RunDevelopmentSessionBrokerFixture(input io.Reader, output io.Writer, composition func(context.Context, *DevelopmentFixtureRuntime) error) error {
	if composition == nil {
		return errors.New("development broker composition missing")
	}
	err := runAuthSessionProcessFixture(input, output, true, composition)
	if err != nil {
		_ = json.NewEncoder(output).Encode(processFixtureReceipt{Stage: "dev_broker_failed", Failure: err.Error()})
	}
	return err
}

func RunDevelopmentSessionFrontendFixture(input io.Reader, output io.Writer, composition func(context.Context, *DevelopmentFixtureRuntime) error) error {
	if composition == nil {
		return errors.New("development frontend composition missing")
	}
	err := runAuthSessionProcessFixtureChecked(input, output, true, composition, func(start processFixtureStart) error {
		return validateDevelopmentFrontendLifetimeStart(start)
	})
	if err != nil {
		_ = json.NewEncoder(output).Encode(processFixtureReceipt{Stage: "dev_frontend_failed", Failure: err.Error()})
	}
	return err
}

func validateDevelopmentFrontendLifetimeStart(start processFixtureStart) error {
	mode := os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED")
	if mode == "" {
		if start.NativeHost != nil {
			return errors.New("native frontend receipt requires its explicit provider")
		}
		return nil
	}
	if mode != "1" || os.Getenv("AKU_DEV_FRONTEND_ENABLED") != "1" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" || os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") != "" || os.Getenv("AKU_DEV_BROKER_ENABLED") != "" || start.NativeHost != nil || start.Grant.Reopen {
		return errors.New("automatic frontend lifetime scoped startup rejected")
	}
	return nil
}

func RunDevelopmentSessionFrontendNativeFixture(input io.Reader, output io.Writer, composition func(context.Context, *DevelopmentFixtureRuntime) error) error {
	if composition == nil || os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") != "1" {
		return errors.New("native frontend requires explicit staged provider")
	}
	err := runAuthSessionProcessFixtureChecked(input, output, true, composition, func(start processFixtureStart) error {
		scenario, scenarioErr := developmentFrontendNativeScenario()
		if scenarioErr != nil {
			return scenarioErr
		}
		return validateDevelopmentFrontendNativeStart(start, scenario)
	})
	if err != nil {
		_ = json.NewEncoder(output).Encode(processFixtureReceipt{Stage: "dev_frontend_native_failed", Failure: err.Error()})
	}
	return err
}

func validateDevelopmentController(start processFixtureStart) error {
	image := start.ControllerImage
	rel, err := filepath.Rel(filepath.Dir(start.Grant.Root), image)
	if err != nil || !filepath.IsAbs(image) || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || !strings.EqualFold(filepath.Base(image), "engine.test.exe") {
		return errors.New("development controller image outside scoped test run")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, start.Grant.Parent.PID)
	if err != nil {
		return errors.New("development controller image unavailable")
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil || !samePath(windows.UTF16ToString(buffer[:size]), image) {
		return errors.New("development controller image changed")
	}
	identity, err := processFixtureProcessIdentity(start.Grant.Parent.PID)
	if err != nil || identity != start.Grant.Parent {
		return errors.New("development controller creation identity changed")
	}
	return nil
}

func bindProcessSession(ctx context.Context, e *Engine, owner *captureruntime.Manager, coordinator *collection.Coordinator, start processFixtureStart, authority, browser func(context.Context) (SyntheticSessionEvidence, error), development bool) error {
	if !development {
		return e.BindSyntheticExclusiveWithBrowserProbe("alpha", authority, browser)
	}
	profileID, err := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
	if err != nil {
		return err
	}
	evidence := func(v SyntheticSessionEvidence, request DevelopmentSessionProbeRequest) DevelopmentSessionEvidence {
		state := DevelopmentSessionAuthUnknown
		if v.Authenticated {
			state = DevelopmentSessionAuthAuthenticated
		}
		return DevelopmentSessionEvidence{
			Version: request.Version, Kind: request.Kind, ProbeID: request.ProbeID, ObservedAt: time.Now().UTC(),
			Domain: "synthetic-fixture", Source: request.Source, ProfileID: profileID,
			AccountID: v.AccountID, AuthState: state, Revision: v.Revision,
		}
	}
	capability := DevelopmentSessionCapability{
		Provider: "synthetic-fixture", StoreScopeID: start.Grant.RootID, ProfileID: profileID,
		ExpectedAccounts: map[domain.Source]string{domain.SourceX: "alpha", domain.SourceInstagram: "alpha"},
		Sources:          []domain.Source{domain.SourceX, domain.SourceInstagram},
	}
	capability.Authority = func(c context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		current, err := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentSessionEvidence{}, errors.New("development profile changed")
		}
		v, err := authority(c)
		if err != nil {
			return DevelopmentSessionEvidence{}, err
		}
		current, err = processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentSessionEvidence{}, errors.New("development profile changed")
		}
		return evidence(v, request), nil
	}
	capability.Browser = func(c context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		current, err := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentBrowserEvidence{}, errors.New("development browser profile changed")
		}
		snapshot, pid := owner.Snapshot(), owner.PID()
		v, err := browser(c)
		if err != nil {
			return DevelopmentBrowserEvidence{}, err
		}
		current, err = processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentBrowserEvidence{}, errors.New("development browser profile changed")
		}
		return DevelopmentBrowserEvidence{DevelopmentSessionEvidence: evidence(v, request), EngineEpoch: e.Epoch(), Generation: snapshot.Generation, PID: pid, Driver: snapshot.Driver}, nil
	}
	return e.StartDevelopmentSession(ctx, owner, coordinator, capability, nil)
}
