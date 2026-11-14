package snapshot_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/snapshot"
)

type document struct {
	Labels map[string]string
}

func (d document) Clone() document {
	copy := document{Labels: make(map[string]string, len(d.Labels))}
	for key, value := range d.Labels {
		copy.Labels[key] = value
	}
	return copy
}

type samples []int

func (s samples) Clone() samples { return append(samples(nil), s...) }

func TestIndependentConsumersAndMutationIsolation(t *testing.T) {
	var documents snapshot.Store[document]
	var measurements snapshot.Store[samples]
	if _, ok := documents.Latest(); ok {
		t.Fatal("zero-value store contains a value")
	}
	input := document{Labels: map[string]string{"status": "first"}}
	documents.Save(input)
	input.Labels["status"] = "caller mutation"
	got, ok := documents.Latest()
	if !ok || got.Labels["status"] != "first" {
		t.Fatalf("incoming value was not isolated: %+v, %v", got, ok)
	}
	got.Labels["status"] = "reader mutation"
	again, _ := documents.Latest()
	if again.Labels["status"] != "first" {
		t.Fatal("reader mutation changed the stored value")
	}
	values := samples{1, 2}
	measurements.Save(values)
	values[0] = 9
	measured, _ := measurements.Latest()
	measured[1] = 8
	measured, _ = measurements.Latest()
	if measured[0] != 1 || measured[1] != 2 {
		t.Fatalf("second consumer lost mutation isolation: %v", measured)
	}
	documents.Save(document{Labels: map[string]string{"status": "replacement"}})
	again, _ = documents.Latest()
	if again.Labels["status"] != "replacement" {
		t.Fatal("Save did not replace the latest value")
	}
	measurements.Save(nil)
	if value, ok := measurements.Latest(); !ok || value != nil {
		t.Fatalf("saved zero value must remain distinct from no value: %v, %v", value, ok)
	}
}

func TestConcurrentAccess(t *testing.T) {
	var store snapshot.Store[document]
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := range 100 {
				input := document{Labels: map[string]string{"value": fmt.Sprintf("%d:%d", worker, iteration)}}
				store.Save(input)
				input.Labels["value"] = "changed after Save"
				got, ok := store.Latest()
				if !ok || got.Labels["value"] == "changed after Save" {
					t.Errorf("invalid concurrent snapshot: %+v, %v", got, ok)
				}
				got.Labels["value"] = "changed after Latest"
			}
		}()
	}
	workers.Wait()
	got, _ := store.Latest()
	if got.Labels["value"] == "changed after Latest" {
		t.Fatal("concurrent reader mutation escaped")
	}
}
