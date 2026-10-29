//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

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
