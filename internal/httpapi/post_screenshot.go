package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

//go:embed post_screenshot_worker.mjs
var postScreenshotWorker []byte

type postScreenshotPart struct {
	Data   string `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type postScreenshotResult struct {
	Parts    []postScreenshotPart `json:"parts"`
	Warnings []string             `json:"warnings"`
	Filename string               `json:"filename"`
}

func (s *Server) createPostScreenshot(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		TimelineID string `json:"timelineId"`
		Theme      string `json:"theme"`
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return badRequest("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return badRequest("invalid screenshot request")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || len(body.TimelineID) == 0 || len(body.TimelineID) > 256 || (body.Theme != "dark" && body.Theme != "light") {
		return badRequest("invalid screenshot request")
	}
	select {
	case s.postScreenshotGate <- struct{}{}:
		defer func() { <-s.postScreenshotGate }()
	default:
		return apiError{Status: http.StatusTooManyRequests, Code: "screenshot_busy", Message: "Another post screenshot is being prepared. Please try again."}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	entry, err := s.store.TimelineItem(ctx, body.TimelineID)
	if err != nil {
		return notFound("timeline post")
	}
	if entry.Evidence == nil {
		return apiError{Status: http.StatusConflict, Code: "screenshot_no_evidence", Message: "Captured source content is unavailable for this post."}
	}
	render := s.postScreenshotRenderer
	if render == nil {
		render = s.renderPostScreenshot
	}
	result, err := render(ctx, entry, body.Theme)
	if err != nil {
		return apiError{Status: http.StatusServiceUnavailable, Code: "screenshot_failed", Message: "The full post screenshot could not be prepared. Media may be unavailable, or the local renderer may have reached its resource limit. Please try again."}
	}
	return writeJSON(w, http.StatusOK, result)
}

// screenshotSnapshot excludes personal memory, comments, AI reasoning and source
// session/run metadata. Only captured public post presentation enters Chrome.
func screenshotSnapshot(entry domain.TimelineItem) map[string]any {
	raw, _ := json.Marshal(entry.Evidence)
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	evidence := make(map[string]any)
	for _, key := range []string{"author", "avatarUrl", "text", "permalink", "publishedAt", "contentKind", "quotedPost", "engagement", "presentation", "media", "attachments"} {
		evidence[key] = block[key]
	}
	return map[string]any{"source": entry.Source, "evidence": evidence, "item": map[string]any{"author": entry.Item.Author, "publishedAt": entry.Item.PublishedAt, "sourceUrl": entry.Item.SourceURL}}
}

func (s *Server) renderPostScreenshot(ctx context.Context, entry domain.TimelineItem, theme string) (result postScreenshotResult, err error) {
	node, err := screenshotNode(s.config.Root, s.config.Dev)
	if err != nil {
		return result, err
	}
	chrome, err := appshell.Discover(ctx, "")
	if err != nil {
		return result, err
	}
	parent := filepath.Join(s.config.Root, "runtime", "post-screenshots")
	if err = os.MkdirAll(parent, 0700); err != nil {
		return result, err
	}
	root, err := os.MkdirTemp(parent, "export-")
	if err != nil {
		return result, err
	}
	removeRoot := true
	defer func() {
		if removeRoot {
			if cleanup := os.RemoveAll(root); cleanup != nil && err == nil {
				err = errors.New("screenshot cleanup failed")
			}
		}
	}()
	snapshot := screenshotSnapshot(entry)
	media, warnings := screenshotMedia(ctx, snapshot)
	result, removeRoot, err = runPostScreenshotProcess(ctx, root, node, chrome.Executable, snapshot, theme, media, warnings)
	return result, err
}

func runPostScreenshotProcess(ctx context.Context, root, node, chrome string, snapshot map[string]any, theme string, media map[string]string, warnings []string) (result postScreenshotResult, removeRoot bool, err error) {
	removeRoot = true
	module, err := embeddedAssets.ReadFile("web/post-screenshot-render.js")
	if err != nil {
		return result, removeRoot, err
	}
	css, err := embeddedAssets.ReadFile("web/styles.css")
	if err != nil {
		return result, removeRoot, err
	}
	for name, content := range map[string][]byte{"worker.mjs": postScreenshotWorker, "render.mjs": module} {
		if err = os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
			return result, removeRoot, err
		}
	}
	input, err := json.Marshal(map[string]any{"entry": snapshot, "css": string(css), "theme": theme, "media": media, "warnings": warnings, "chrome": chrome, "root": root})
	if err != nil {
		return result, removeRoot, err
	}
	cmd := exec.Command(node, filepath.Join(root, "worker.mjs"))
	cmd.Stdin = bytes.NewReader(input)
	output := &screenshotBuffer{limit: 48 << 20}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	cmd.Env = append(os.Environ(), "TEMP="+root, "TMP="+root, "TMPDIR="+root)
	owner, err := appshell.StartOwnedCommand(cmd)
	if err != nil {
		// Owned startup may return an unverified-cleanup error. Retain this
		// private root rather than deleting files a surviving child may use.
		return result, false, err
	}
	select {
	case err = <-owner.Done():
		if err != nil {
			// A normal worker failure still has verified Job cleanup. Other
			// errors may be cleanup failures and must retain the private root.
			var exited *exec.ExitError
			removeRoot = errors.As(err, &exited)
		}
	case <-ctx.Done():
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		err = owner.CloseForRetry(cleanup)
		stop()
		if err != nil {
			removeRoot = false
		}
		if err == nil {
			err = ctx.Err()
		}
	}
	if err != nil {
		return result, removeRoot, err
	}
	if err = json.Unmarshal(output.Bytes(), &result); err != nil || len(result.Parts) == 0 {
		return result, removeRoot, errors.New("invalid screenshot renderer output")
	}
	result.Filename = "akubrowser-post-" + time.Now().UTC().Format("20060102-150405")
	return result, removeRoot, nil
}

type screenshotBuffer struct {
	bytes.Buffer
	limit int
}

func (b *screenshotBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("screenshot output limit exceeded")
	}
	return b.Buffer.Write(p)
}

func screenshotNode(root string, dev bool) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "node"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dirs := []string{filepath.Join(filepath.Dir(executable), "headless-worker")}
	if dev {
		dirs = append(dirs, filepath.Join(root, "runtime", "dev", "headless-worker"))
	}
	for _, dir := range dirs {
		var pin struct {
			Protocol int    `json:"protocol"`
			SHA      string `json:"nodeSha256"`
		}
		raw, e := os.ReadFile(filepath.Join(dir, "node.pin.json"))
		if e != nil {
			continue
		}
		if json.Unmarshal(raw, &pin) != nil || pin.Protocol != 1 || len(pin.SHA) != 64 {
			continue
		}
		binary := filepath.Join(dir, name)
		f, e := os.Open(binary)
		if e != nil {
			continue
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e == nil && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), pin.SHA) {
			return binary, nil
		}
	}
	return "", errors.New("packaged screenshot Node runtime is unavailable")
}
