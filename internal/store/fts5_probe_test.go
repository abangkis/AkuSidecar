package store

import (
	"context"
	"database/sql"
	"testing"
)

// TestModerncSQLiteFTS5Support is the focused capability probe for the
// driver/version pinned by this repository. The v13 migration depends on the
// bundled FTS5 tokenizer, MATCH query, and BM25 ranking being available.
func TestModerncSQLiteFTS5Support(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:fts5-probe?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE fts5_probe USING fts5(title,body,tokenize='unicode61')`); err != nil {
		t.Fatalf("modernc SQLite FTS5 is unavailable: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO fts5_probe(title,body) VALUES('local memory','deterministic recall')`); err != nil {
		t.Fatal(err)
	}
	var rank float64
	if err := db.QueryRowContext(ctx, `SELECT bm25(fts5_probe,10.0,1.0) FROM fts5_probe WHERE fts5_probe MATCH 'memory'`).Scan(&rank); err != nil {
		t.Fatalf("FTS5 MATCH/BM25 unavailable: %v", err)
	}
}
