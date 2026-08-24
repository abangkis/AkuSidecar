// semantic-replay is a local, read-only assessment tool. It never starts a
// provider and never opens the normal writable runtime path.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func main() {
	database := flag.String("database", "", "path to an existing AkuSidecar SQLite database")
	limit := flag.Int("limit", 100, "maximum completed sessions to inspect (1-500)")
	flag.Parse()
	if *database == "" {
		fatal("-database is required; this tool does not infer or modify the runtime database")
	}
	path, err := filepath.Abs(*database)
	if err != nil {
		fatal(fmt.Sprintf("resolve database path: %v", err))
	}
	state, err := store.OpenReadOnly(path)
	if err != nil {
		fatal(err.Error())
	}
	defer state.Close()
	report, err := eventengine.AnalyzeSemanticReplay(context.Background(), state, *limit)
	if err != nil {
		fatal(err.Error())
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "semantic-replay:", message)
	os.Exit(2)
}
