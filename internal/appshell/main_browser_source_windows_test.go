//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestMainBrowserSourceClientPreservesHTTPIdentityCodes(t *testing.T) {
	for _, role := range []string{"reader", "collector", "unknown"} {
		for _, status := range []int{100, 400, 404, 500, 599} {
			code := fmt.Sprintf("native_source_evidence_%s_identity_http_status_%d", role, status)
			t.Run(code, func(t *testing.T) {
				c, writer, writes := syntheticMainBrowserClient(t)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() { _, err := c.CallSource(ctx, "instagram", "source_check", map[string]any{}); done <- err }()
				var request struct {
					ID int64 `json:"id"`
				}
				select {
				case raw := <-writes:
					if err := json.Unmarshal(raw, &request); err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if _, err := fmt.Fprintf(writer, "{\"id\":%d,\"ok\":false,\"error\":{\"code\":%q}}\n", request.ID, code); err != nil {
					t.Fatal(err)
				}
				var failure *MainBrowserError
				if err := <-done; !errors.As(err, &failure) || failure.Code != code {
					t.Fatalf("source diagnostic changed across client decode: %v", err)
				}
			})
		}
	}
}

func TestMainBrowserSourceWireIsSeparateFromX(t *testing.T) {
	c, writer, writes := syntheticMainBrowserClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.CallSource(ctx, "instagram", "source_check", map[string]any{}); done <- err }()
	var request struct {
		ID        int64          `json:"id"`
		Op        string         `json:"op"`
		Source    string         `json:"source"`
		Operation string         `json:"operation"`
		Params    map[string]any `json:"params"`
	}
	select {
	case raw := <-writes:
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if request.Op != "main_source" || request.Source != "instagram" || request.Operation != "source_check" {
		t.Fatalf("source lane mismatch: %+v", request)
	}
	if _, err := io.WriteString(writer, fmt.Sprintf("{\"id\":%d,\"ok\":true,\"result\":{}}\n", request.ID)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := c.CallSource(ctx, "x", "source_check", map[string]any{}); err == nil {
		t.Fatal("X admitted to source lane")
	}
	select {
	case <-writes:
		t.Fatal("rejected source wrote protocol bytes")
	default:
	}
}
