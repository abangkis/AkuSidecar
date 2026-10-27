package uireaderbroker

import (
	"embed"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed manifest.json content.js service-worker.js
var assets embed.FS

// Materialize writes a fresh instance-local copy of the trusted UI broker.
// Only its exact loopback origin changes; extension identity, native host,
// foreground checks and the message protocol remain the packaged values.
func Materialize(directory, origin string) error {
	u, err := url.Parse(origin)
	if err != nil || u == nil {
		return errors.New("isolated UI broker scope rejected")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || port < 1024 || port > 65535 || u.Port() != strconv.Itoa(port) || u.String() != origin || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return errors.New("isolated UI broker scope rejected")
	}
	if os.Mkdir(directory, 0700) != nil {
		return errors.New("fresh isolated UI broker directory required")
	}
	raw, _ := assets.ReadFile("manifest.json")
	var manifest map[string]json.RawMessage
	if json.Unmarshal(raw, &manifest) != nil {
		return errors.New("packaged UI broker manifest invalid")
	}
	permission, _ := json.Marshal([]string{origin + "/*"})
	manifest["host_permissions"] = permission
	var scripts []map[string]json.RawMessage
	if json.Unmarshal(manifest["content_scripts"], &scripts) != nil || len(scripts) != 1 {
		return errors.New("packaged UI broker scripts invalid")
	}
	scripts[0]["matches"] = permission
	manifest["content_scripts"], _ = json.Marshal(scripts)
	manifestBytes, _ := json.MarshalIndent(manifest, "", " ")
	worker, _ := assets.ReadFile("service-worker.js")
	const original = `const uiOrigins = new Set(["http://127.0.0.1:11122", "http://localhost:11122"]);`
	if strings.Count(string(worker), original) != 1 {
		return errors.New("packaged UI broker origin policy changed")
	}
	origins, _ := json.Marshal([]string{origin})
	worker = []byte(strings.Replace(string(worker), original, "const uiOrigins = new Set("+string(origins)+");", 1))
	content, _ := assets.ReadFile("content.js")
	for name, data := range map[string][]byte{"manifest.json": manifestBytes, "service-worker.js": worker, "content.js": content} {
		f, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("isolated UI broker asset unavailable")
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			return errors.New("isolated UI broker asset incomplete")
		}
	}
	return nil
}
