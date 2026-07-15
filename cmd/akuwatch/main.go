package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

func main() {
	root, err := findRoot()
	check(err)
	check(ensureGoEnvironment(root))
	runtimeDir := filepath.Join(root, "runtime", "dev")
	check(os.MkdirAll(runtimeDir, 0o755))
	binary := filepath.Join(runtimeDir, "aku-sidecar.exe")
	current := snapshot(root)
	child := buildAndStart(root, binary)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()
	fmt.Printf("AkuSidecar Go watcher\nRoot: %s\nActive executable: %s\nPolicy: changes wait for the active session to finish before restart\n", root, binary)
	for {
		select {
		case <-signals:
			stop(child)
			return
		case <-ticker.C:
			next := snapshot(root)
			if next == current {
				continue
			}
			fmt.Println("Source change detected.")
			for !safeToRestart() {
				fmt.Println("Active session detected; deferring rebuild.")
				time.Sleep(2 * time.Second)
			}
			stop(child)
			child = buildAndStart(root, binary)
			current = next
		}
	}
}

func buildAndStart(root, binary string) *exec.Cmd {
	build := exec.Command("go", "build", "-o", binary, "./cmd/akusidecar")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	fmt.Println("Building AkuSidecar Go runtime...")
	check(build.Run())
	child := exec.Command(binary, "--config", filepath.Join(root, "config", "sidecar.json"), "--dev")
	child.Dir = root
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	check(child.Start())
	fmt.Printf("Started PID %d\n", child.Process.Pid)
	return child
}
func stop(child *exec.Cmd) {
	if child == nil || child.Process == nil {
		return
	}
	_ = child.Process.Kill()
	_, _ = child.Process.Wait()
}

func ensureGoEnvironment(root string) error {
	cacheRoot := filepath.Join(filepath.Dir(root), ".go-cache")
	defaults := map[string]string{
		"GOCACHE":    filepath.Join(cacheRoot, "build"),
		"GOMODCACHE": filepath.Join(cacheRoot, "mod"),
		"GOTMPDIR":   filepath.Join(cacheRoot, "tmp"),
	}
	for key, value := range defaults {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
		if err := os.MkdirAll(os.Getenv(key), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", key, err)
		}
	}
	return nil
}

func safeToRestart() bool {
	client := http.Client{Timeout: 700 * time.Millisecond}
	response, err := client.Get("http://127.0.0.1:47821/api/sessions/active")
	if err != nil {
		return true
	}
	defer response.Body.Close()
	var value struct {
		Session any `json:"session"`
	}
	if json.NewDecoder(response.Body).Decode(&value) != nil {
		return true
	}
	return value.Session == nil
}

func snapshot(root string) string {
	var entries []string
	_ = filepath.Walk(root, func(value string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relative, _ := filepath.Rel(root, value)
		if info.IsDir() {
			if relative == "runtime" || relative == ".git" || strings.HasPrefix(relative, "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		extension := strings.ToLower(filepath.Ext(value))
		if extension == ".go" || extension == ".json" || extension == ".html" || extension == ".css" || extension == ".js" {
			entries = append(entries, fmt.Sprintf("%s:%d:%d", relative, info.Size(), info.ModTime().UnixNano()))
		}
		return nil
	})
	sort.Strings(entries)
	return strings.Join(entries, "|")
}
func findRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found")
		}
		current = parent
	}
}
func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
