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
)

// RunDevelopmentSessionFixture is a concrete synthetic provider composition,
// dispatched by the actual main binary before ordinary startup. The private
// controller grant still validates parent creation/image and scoped fresh profile.
func RunDevelopmentSessionFixture(input io.Reader, output io.Writer) error {
	return runAuthSessionProcessFixture(input, output, true, nil)
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
	err := runAuthSessionProcessFixture(input, output, true, composition)
	if err != nil {
		_ = json.NewEncoder(output).Encode(processFixtureReceipt{Stage: "dev_frontend_failed", Failure: err.Error()})
	}
	return err
}

func RunDevelopmentSessionFrontendNativeFixture(input io.Reader, output io.Writer, composition func(context.Context, *DevelopmentFixtureRuntime) error) error {
	if composition == nil || os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") != "1" {
		return errors.New("native frontend requires explicit staged provider")
	}
	err := runAuthSessionProcessFixtureChecked(input, output, true, composition, func(start processFixtureStart) error {
		if start.NativeHost == nil || start.Grant.Reopen || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" || start.NativeHost.Pipe != os.Getenv("AKU_BROKER_FIXTURE_PIPE") {
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
	evidence := func(v SyntheticSessionEvidence, source domain.Source) DevelopmentSessionEvidence {
		return DevelopmentSessionEvidence{Domain: "synthetic-fixture", Source: source, ProfileID: profileID, AccountID: v.AccountID, Authenticated: v.Authenticated, Revision: v.Revision}
	}
	capability := DevelopmentSessionCapability{Provider: "synthetic-fixture", ProfileID: profileID, AccountID: "alpha", Sources: []domain.Source{domain.SourceX, domain.SourceInstagram}}
	capability.Authority = func(c context.Context, source domain.Source) (DevelopmentSessionEvidence, error) {
		current, err := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentSessionEvidence{}, errors.New("development profile changed")
		}
		v, err := authority(c)
		return evidence(v, source), err
	}
	capability.Browser = func(c context.Context, source domain.Source) (DevelopmentBrowserEvidence, error) {
		current, err := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if err != nil || current != profileID {
			return DevelopmentBrowserEvidence{}, errors.New("development browser profile changed")
		}
		snapshot, pid := owner.Snapshot(), owner.PID()
		v, err := browser(c)
		return DevelopmentBrowserEvidence{DevelopmentSessionEvidence: evidence(v, source), EngineEpoch: e.Epoch(), Generation: snapshot.Generation, PID: pid, Driver: snapshot.Driver}, err
	}
	return e.StartDevelopmentSession(ctx, owner, coordinator, capability, nil)
}
