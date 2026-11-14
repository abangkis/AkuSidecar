package recording

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

func TestIndependentConsumersRetainTheirOwnVocabularyAndState(t *testing.T) {
	first := New("document_index")
	second := New("asset_publish")
	first.Advance("parse", d.Observed[time.Duration]{Value: 2 * time.Millisecond, Known: true})
	first.Completed("parse")
	first.Fail(d.Location{Operation: "document_index", Phase: "parse"}, "invalid_document")
	first.Incomplete()
	second.Advance("upload", d.Observed[time.Duration]{Known: false})
	second.Cleanup("complete", "none")
	second.Delivery("saved", "none")
	second.Disposition("available")
	a, b := first.Snapshot(), second.Snapshot()
	if a.Location.Operation != "document_index" || a.Completed != "parse" || a.Complete {
		t.Fatal("first consumer state changed")
	}
	if b.Location.Operation != "asset_publish" || b.Location.Phase != "upload" || b.Cause != nil || !b.Complete {
		t.Fatal("recorder shared product vocabulary or state")
	}
	if b.Elapsed.Known || b.Disposition != "available" || b.Cleanup.State != "complete" || b.Delivery.State != "saved" {
		t.Fatal("unknown timing or independent settlement was lost")
	}
}
