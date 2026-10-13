//go:build windows && auth_session_fixture

package readerbroker

import (
	"context"
	"strings"
	"testing"
)

func TestReaderFixturePipeRejectsProductionAndUnboundedAddresses(t *testing.T) {
	good := `\\.\pipe\AkuBrowser.fixture-reader.` + strings.Repeat("a", 32)
	if ValidateFixturePipe(good) != nil {
		t.Fatal("isolated pipe rejected")
	}
	for _, pipe := range []string{PipeName, good + "x", `\\remote\pipe\AkuBrowser.fixture-reader.` + strings.Repeat("a", 32), `\\.\pipe\AkuBrowser.fixture-reader.` + strings.Repeat("A", 32), ""} {
		if ValidateFixturePipe(pipe) == nil {
			t.Fatal("unsafe fixture address accepted")
		}
		if RunFixtureClient(context.Background(), Request{}, pipe).Message != "invalid isolated fixture pipe" {
			t.Fatal("client did not reject before OS operations")
		}
		if ServeFixture(context.Background(), pipe, nil, nil) == nil {
			t.Fatal("server did not reject before listener creation")
		}
	}
}
