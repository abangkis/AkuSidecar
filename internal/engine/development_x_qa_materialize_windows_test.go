//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func xQAMaterializeFixture(t *testing.T) (XApplicationQAMaterialization, string) {
	t.Helper()
	project, err := xRetainedQAProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(project, "build", "x-application-qa-prepared")
	if err = os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, strings.TrimPrefix(domain.NewID("prep"), "prep_"))
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resolved, e := filepath.EvalSymlinks(root)
		if e != nil || resolved != root || filepath.Dir(resolved) != parent {
			t.Error("unsafe materialization test cleanup")
			return
		}
		if e = filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			return xQAOrdinary(path, info.IsDir())
		}); e != nil {
			t.Error("aliased test cleanup tree")
			return
		}
		if e = os.RemoveAll(resolved); e != nil {
			t.Error(e)
		}
	})
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical QA helper required")
	}
	synthetic, err := WriteXApplicationQASyntheticPlan(root, node, 28173)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(synthetic)
	if err != nil {
		t.Fatal(err)
	}
	var p XApplicationQAPlan
	if json.Unmarshal(raw, &p) != nil {
		t.Fatal("synthetic assets unavailable")
	}
	return XApplicationQAMaterialization{Schema: "aku.x.application-qa-materialize.v1", Parent: root, Port: 28174, Node: p.Node, Chrome: p.Chrome, Driver: p.Driver}, root
}
func TestXQAMaterializationDeferredRetainedSourceAndNoDatabase(t *testing.T) {
	req, root := xQAMaterializeFixture(t)
	raw, _ := json.Marshal(req)
	requestPath := filepath.Join(root, "materialize-request.json")
	if os.WriteFile(requestPath, raw, 0600) != nil {
		t.Fatal("request write failed")
	}
	path, err := MaterializeXApplicationQAPlanFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var p XApplicationQAPlan
	if json.Unmarshal(data, &p) != nil {
		t.Fatal("plan invalid")
	}
	if p.ReaderSource != "retained-qa" || p.Reader != (XApplicationQAProfile{}) || p.ProjectRoot == "" {
		t.Fatal("retained source was resolved or altered")
	}
	for _, absent := range []string{p.Database, filepath.Join(p.Root, "profiles", "reader")} {
		if _, err = os.Stat(absent); !os.IsNotExist(err) {
			t.Fatal("preparation touched database or reader")
		}
	}
	selection, err := openXQASelection(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer selection.close()
	if selection.owner.reader.Directory != "" || selection.owner.retainedProject != p.ProjectRoot {
		t.Fatal("selection acquired retained authority")
	}
	if hash, err := XApplicationQAPlanSHA256(path); err != nil || len(hash) != 64 {
		t.Fatal("plan fingerprint unavailable")
	}
}
func TestXQAMaterializationRejectsUntrustedSelection(t *testing.T) {
	for _, fault := range []string{"schema", "port", "asset", "parent", "consent", "request-path"} {
		t.Run(fault, func(t *testing.T) {
			req, root := xQAMaterializeFixture(t)
			switch fault {
			case "schema":
				req.Schema = "wrong"
			case "port":
				req.Port = 11122
			case "asset":
				req.Driver.SHA256 = strings.Repeat("a", 64)
			case "parent":
				req.Parent = filepath.Dir(root)
			}
			raw, _ := json.Marshal(req)
			if fault == "consent" {
				raw = append(raw[:len(raw)-1], []byte(`,"allowOneSessionDerivation":true}`)...)
			}
			var err error
			if fault == "request-path" {
				_, err = MaterializeXApplicationQAPlanFile(filepath.Join(root, "plan.json"))
			} else {
				_, err = MaterializeXApplicationQAPlan(raw)
			}
			if err == nil {
				t.Fatal("invalid preparation accepted")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatal("invalid request created another instance")
			}
		})
	}
}
