//go:build windows

package appshell

import (
	"context"
	"os"
	"testing"
)

func TestSplitUIPinnedArtifactReadOnly(t *testing.T) {
	path := os.Getenv("AKUBROWSER_TEST_CFT_PATH")
	if path == "" {
		t.Skip("optional read-only packaged CfT fixture")
	}
	result, err := DiscoverSplitUI(context.Background(), "unused.exe", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("validated CfT version=%s source=%s", result.Version, result.Source)
}
