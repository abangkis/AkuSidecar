// Command sqlite-driver-compat is an accountless database fixture, not an app
// entry. The test helper compiles identical source with the old and new drivers.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := os.Getenv("AKU_SQLITE_UPGRADE_ROOT")
	if len(os.Args) != 3 || !filepath.IsAbs(root) || filepath.Clean(root) != root || !regexp.MustCompile(`^run-sqlite-upgrade-[a-f0-9]{32}$`).MatchString(filepath.Base(root)) || os.Args[2] != filepath.Join(root, "compat.db") {
		return fmt.Errorf("explicit synthetic compatibility scope required")
	}
	op := os.Args[1]
	path := os.Args[2]
	if op != "old-seed" && op != "new-read-write" && op != "old-reopen" {
		return fmt.Errorf("synthetic operation rejected")
	}
	if _, err := os.Stat(path); op == "old-seed" && !os.IsNotExist(err) {
		return fmt.Errorf("fresh compatibility database required")
	}
	state, err := store.Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false))
	if err != nil {
		return err
	}
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		state.Close()
		return err
	}
	switch op {
	case "old-seed":
		settings.SingleImageFit = "contain"
		err = state.SaveSettings(context.Background(), settings)
	case "new-read-write":
		if settings.SingleImageFit != "contain" {
			state.Close()
			return fmt.Errorf("new driver lost old setting")
		}
		settings.SingleImageFit = "cover"
		err = state.SaveSettings(context.Background(), settings)
	case "old-reopen":
		if settings.SingleImageFit != "cover" {
			state.Close()
			return fmt.Errorf("old driver lost new setting")
		}
	}
	closeErr := state.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	closeErr = db.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"operation": op, "sqlite": version, "schema": store.SchemaVersion, "settingsPreserved": true})
}
