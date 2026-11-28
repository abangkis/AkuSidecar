package operational

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"testing"
	"time"
)

func TestProblemProjectsWrappedErrorsAndIssuedIDs(t *testing.T) {
	const private = "private-path-account-and-token"
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{context.Canceled, "cancelled"}, {context.DeadlineExceeded, "deadline"},
		{fs.ErrPermission, "permission_denied"}, {sql.ErrNoRows, "not_found"},
		{fs.ErrNotExist, "not_found"}, {fs.ErrExist, "already_exists"}, {fs.ErrInvalid, "invalid"}, {errors.New(private), "unavailable"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			var out bytes.Buffer
			id := "ai_feedback_" + strings.Repeat("0a", 16)
			Problem(log.New(&out, "", 0), "ai_feedback", "load_item", "failed", fmt.Errorf("%s: %w", private, tc.err), id)
			var row event
			if !strings.HasPrefix(out.String(), "sidecar_activity ") || strings.Contains(out.String(), private) || json.Unmarshal(bytes.TrimPrefix(out.Bytes(), []byte("sidecar_activity ")), &row) != nil || row.ID != id || row.Reason != tc.reason || row.Outcome != "failed" {
				t.Fatal("invalid projection", out.String())
			}
		})
	}
	for _, id := range []string{private, "session_" + strings.Repeat("z", 32), "token_" + strings.Repeat("a", 32)} {
		if OpaqueID(id) != "" {
			t.Errorf("accepted non-issued ID %q", id)
		}
	}
	if Provider(private) != "other" || DiscoverySource(private) != "other" || Milliseconds(1250*time.Millisecond) != 1250 {
		t.Fatal("catalog or duration projection changed")
	}
	var absent *log.Logger
	Problem(absent, "http", "serve", "failed", errors.New(private), "")
}
