package diagnostictrace

import (
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
	"testing"
	"time"
)

func TestFirstCauseAndIndependentSettlement(t *testing.T) {
	r := New("operation")
	r.Advance("read", d.Observed[time.Duration]{Value: time.Millisecond, Known: true})
	r.Fail(d.Location{Operation: "operation", Phase: "read"}, "failed")
	r.Fail(d.Location{Operation: "operation", Phase: "cleanup"}, "failed_again")
	r.Cleanup("uncertain", "close")
	r.Delivery("unavailable", "EOF")
	s := r.Snapshot()
	s.Cause.Condition = "modified"
	if r.Snapshot().Cause.Condition != "failed" {
		t.Fatal("mutable snapshot")
	}
	if s.Cleanup.State != "uncertain" || s.Delivery.State != "unavailable" {
		t.Fatal("settlements collapsed")
	}
}
