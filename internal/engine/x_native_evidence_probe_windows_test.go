//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type xNativeMetadataCandidate struct {
	Alias        string `json:"alias"`
	Operation    string `json:"operation"`
	Method       string `json:"method"`
	Status       *int   `json:"status"`
	Ambiguous    bool   `json:"ambiguous"`
	HasExtraInfo *bool  `json:"hasExtraInfo"`
	Cache        struct {
		ServedFromCache   *bool `json:"servedFromCache"`
		FromDiskCache     *bool `json:"fromDiskCache"`
		FromServiceWorker *bool `json:"fromServiceWorker"`
		FromPrefetchCache *bool `json:"fromPrefetchCache"`
	} `json:"cache"`
	Completion string `json:"completion"`
	Bytes      *int64 `json:"bytes"`
	TimingMS   struct {
		Response   *int `json:"response"`
		Completion *int `json:"completion"`
	} `json:"timingMs"`
}

type xNativeMetadataReport struct {
	Schema              string                     `json:"schema"`
	Coverage            string                     `json:"coverage"`
	ExtraInfoCoverage   string                     `json:"extraInfoCoverage"`
	ObservationWindowMS int                        `json:"observationWindowMs"`
	CandidateCount      int                        `json:"candidateCount"`
	DroppedCandidates   int                        `json:"droppedCandidates"`
	EventLimitReached   bool                       `json:"eventLimitReached"`
	Candidates          []xNativeMetadataCandidate `json:"candidates"`
	CanAuthorize        bool                       `json:"canAuthorize"`
}

func decodeXNativeMetadata(raw []byte) (xNativeMetadataReport, error) {
	var result xNativeMetadataReport
	invalid := errors.New("native metadata projection rejected")
	if len(raw) == 0 || len(raw) > 65536 {
		return result, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF ||
		result.Schema != "aku.x.native-metadata-observation.v1" ||
		result.Coverage != "partial_single_page_home_reload" || result.ExtraInfoCoverage != "not_observed" ||
		result.ObservationWindowMS != 20000 || result.CanAuthorize || result.CandidateCount != len(result.Candidates) ||
		result.CandidateCount > 64 || result.DroppedCandidates < 0 || result.DroppedCandidates > 10000 {
		return xNativeMetadataReport{}, invalid
	}
	operations := map[string]bool{"other": true, "graphql:other": true, "graphql:Account": true,
		"rest:other": true, "rest:account-settings": true, "rest:verify-credentials": true, "rest:account-multi-list": true,
		"graphql:CurrentUser": true, "graphql:HomeLatestTimeline": true, "graphql:HomeTimeline": true,
		"graphql:UserByRestId": true, "graphql:UserByScreenName": true, "graphql:UsersByRestIds": true, "graphql:Viewer": true}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true, "OTHER": true}
	aliases := map[string]bool{}
	for _, item := range result.Candidates {
		if !regexp.MustCompile(`^r([1-9]|[1-5][0-9]|6[0-4])$`).MatchString(item.Alias) || aliases[item.Alias] ||
			!operations[item.Operation] || !methods[item.Method] ||
			item.Status != nil && (*item.Status < 100 || *item.Status > 599 || item.Ambiguous) ||
			item.Bytes != nil && (*item.Bytes < 0 || *item.Bytes > 100000000) ||
			item.Completion != "complete" && item.Completion != "failed" && item.Completion != "incomplete" {
			return xNativeMetadataReport{}, invalid
		}
		aliases[item.Alias] = true
		for _, timing := range []*int{item.TimingMS.Response, item.TimingMS.Completion} {
			if timing != nil && (*timing < 0 || *timing > 20000 || *timing%250 != 0) {
				return xNativeMetadataReport{}, invalid
			}
		}
	}
	return result, nil
}

func TestXNativeMetadataProbeOffline(t *testing.T) {
	valid := []byte(`{"schema":"aku.x.native-metadata-observation.v1","coverage":"partial_single_page_home_reload","extraInfoCoverage":"not_observed","observationWindowMs":20000,"candidateCount":0,"droppedCandidates":0,"candidates":[],"canAuthorize":false}`)
	if _, err := decodeXNativeMetadata(valid); err != nil {
		t.Fatal("valid non-authorizing metadata was rejected")
	}
	for _, field := range []string{`,"rawURL":"private-value"`, `,"accountId":"private-value"`, `,"headers":{}`} {
		bad := append(append([]byte{}, valid[:len(valid)-1]...), []byte(field+"}")...)
		if _, err := decodeXNativeMetadata(bad); err == nil {
			t.Fatal("private or unknown fields crossed the report boundary")
		}
	}
	var changed map[string]any
	_ = json.Unmarshal(valid, &changed)
	changed["canAuthorize"] = true
	bad, _ := json.Marshal(changed)
	if _, err := decodeXNativeMetadata(bad); err == nil {
		t.Fatal("metadata became authorization")
	}
	changed["canAuthorize"] = false
	changed["candidateCount"] = 1
	item := xNativeMetadataCandidate{Alias: "r1", Operation: "graphql:Viewer", Method: "GET", Completion: "complete"}
	changed["candidates"] = []xNativeMetadataCandidate{item}
	goodCandidate, _ := json.Marshal(changed)
	if _, err := decodeXNativeMetadata(goodCandidate); err != nil {
		t.Fatal("sanitized candidate was rejected")
	}
	item.Operation = "graphql:PrivateAccountValue"
	changed["candidates"] = []xNativeMetadataCandidate{item}
	bad, _ = json.Marshal(changed)
	if _, err := decodeXNativeMetadata(bad); err == nil {
		t.Fatal("unapproved operation values crossed report boundary")
	}
}

// Only the explicit helper can start this test. No Store or controller is used.
func TestXNativeMetadataProbe(t *testing.T) {
	if os.Getenv("AKU_X_NATIVE_METADATA_ENABLED") != "1" {
		t.Skip("native X probe requires explicit manual-login QA helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("probe source root unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	openOutput := func(variable, suffix string) *os.File {
		path := os.Getenv(variable)
		if !samePath(filepath.Dir(path), qaParent) || !regexp.MustCompile(`^x-native-metadata-[a-f0-9]{32}\.`+suffix+`$`).MatchString(filepath.Base(path)) {
			t.Fatal("probe output scope invalid")
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal("probe output unavailable")
		}
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	progress := openOutput("AKU_X_NATIVE_METADATA_PROGRESS_PATH", `progress\.jsonl`)
	stage := func(value string) {
		if _, err := progress.WriteString("{\"stage\":\"" + value + "\"}\n"); err != nil {
			t.Fatal("probe progress unavailable")
		}
		_ = progress.Sync()
		t.Log("X native QA stage:", value)
	}
	defer func() {
		if t.Failed() {
			stage("failed")
		}
	}()
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("probe scope unavailable")
	}
	caseRoot := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-native-metadata-"+hex.EncodeToString(nonce[:]))
	if err := os.Mkdir(caseRoot, 0700); err != nil {
		t.Fatal("new probe root unavailable")
	}
	config := chromeFixtureConfig{
		NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"),
		Root: caseRoot, Origin: server.URL,
		DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-native-metadata-probe-driver.mjs"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("native probe bridge startup failed:", err)
	}
	registerChromeFixtureBridge(bridge)
	var rootHandle windows.Handle
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			t.Error("owned native QA Job cleanup unverified")
		}
		if rootHandle != 0 {
			state, waitErr := windows.WaitForSingleObject(rootHandle, 5000)
			if waitErr != nil || state != windows.WAIT_OBJECT_0 {
				t.Error("owned native QA Chrome exit unverified")
			}
			_ = windows.CloseHandle(rootHandle)
		}
		if !t.Failed() {
			stage("cleanup_verified")
		}
	})
	if err := bridge.assertBridgeOnly(ctx); err != nil {
		t.Fatal("Chrome started before Job assignment")
	}
	stage("bridge_ready")
	owner, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("owned QA Chrome launch failed:", err)
	}
	rootHandle, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
	if err != nil {
		t.Fatal("owned QA Chrome handle unavailable")
	}
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
	stage("manual_login")
	deadline := time.Now().Add(5 * time.Minute)
	ready := false
	for time.Now().Before(deadline) {
		var state struct {
			Ready bool `json:"ready"`
		}
		if err := bridge.rpc(ctx, "login_state", map[string]any{"owner": owner.owner}, &state); err != nil {
			t.Fatal("manual-login UI check failed:", err)
		}
		if state.Ready {
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("probe login deadline expired")
		case <-time.After(2 * time.Second):
		}
	}
	if !ready {
		t.Fatal("signed-in Home UI not detected within five minutes")
	}
	stage("observing")
	var raw json.RawMessage
	if err := bridge.rpc(ctx, "start_observation", map[string]any{"owner": owner.owner}, &raw); err != nil {
		t.Fatal("native metadata observation failed:", err)
	}
	report, err := decodeXNativeMetadata(raw)
	if err != nil {
		t.Fatal("native metadata report failed its privacy boundary")
	}
	output := openOutput("AKU_X_NATIVE_METADATA_REPORT_PATH", `report\.json`)
	if err := json.NewEncoder(output).Encode(report); err != nil {
		t.Fatal("sanitized native report write failed")
	}
	_ = output.Sync()
	t.Logf("Sanitized native sample: %d candidates, %d dropped; partial coverage; canAuthorize=false", report.CandidateCount, report.DroppedCandidates)
	stage("complete")
}
