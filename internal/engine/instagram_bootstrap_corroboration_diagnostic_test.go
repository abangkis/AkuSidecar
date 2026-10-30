package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

const corroborationEngineReply = `{"schema":"aku.instagram-bootstrap-corroboration.v1","reason":"corroborated","bodyBound":true,"documentBound":true,"serverFresh":true,"idMatched":true,"cookieMatched":true,"scanComplete":true,"qualified":false,"cleanup":"verified"}`

func TestInstagramBootstrapCorroborationSharesAdmissionAndPreservesState(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	before, err := e.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	action := func(context.Context) (json.RawMessage, error) {
		calls++
		if e.operation.TryLock() {
			e.operation.Unlock()
			t.Fatal("normal work barrier not held")
		}
		return json.RawMessage(corroborationEngineReply), nil
	}
	if _, err = e.RunInstagramBootstrapCorroborationDiagnostic(context.Background(), action); err != nil || calls != 1 {
		t.Fatal(err)
	}
	if _, err = e.RunInstagramBootstrapDiagnostic(context.Background(), action); err == nil || calls != 1 {
		t.Fatal("passive lane bypassed shared one-shot")
	}
	if _, err = e.RunInstagramReaderDiagnostic(context.Background(), action); err == nil || calls != 1 {
		t.Fatal("reader lane bypassed shared one-shot")
	}
	after, err := e.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !e.nativeDerivationHold.Held("x") || !e.nativeDerivationHold.Held("instagram") {
		t.Fatal("operator state changed")
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		if d.state.owner != nil || d.state.startUsed || d.state.generation != 0 {
			t.Fatal("diagnostic started native collection")
		}
	}
}
