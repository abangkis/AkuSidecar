//go:build auth_session_dev

package engine

import (
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"testing"
)

func TestSourceNativeFailureRetainsSafeFirstCause(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  string
	}{
		{sourceNativeFail(errors.New("private-cookie-and-profile"), "evidence", "pair"), "native_source_evidence_pair"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_session_required"}), "native_source_session_required"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_secret_private-cookie-and-profile"}), "native_source_wire_unavailable"},
	} {
		e, _ := testEngine(t)
		e.config.Capture.InstagramMode = config.XCaptureConcurrent
		d := e.dailyController(domain.SourceInstagram)
		d.rememberFailure(test.cause, "capture")
		d.rememberFailure(sourceNativeFail(nil, "cleanup", "unverified"), "cleanup")
		if got := d.status().FailureCode; got != test.want {
			t.Fatalf("diagnostic=%s want %s", got, test.want)
		}
		if strings.Contains(fmt.Sprintf("%v %#v", test.cause, test.cause), "private-cookie") {
			t.Fatal("private cause escaped diagnostic")
		}
	}
	cause := errors.New("private-cookie-and-profile")
	err := sourceNativeFail(cause, "ownership", "changed")
	if !errors.Is(err, cause) {
		t.Fatal("private causal chain lost")
	}
}
