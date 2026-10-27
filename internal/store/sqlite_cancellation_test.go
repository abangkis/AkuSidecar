package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// Exercise the public driver API only. The race is intentionally stressed, not
// claimed to have deterministic scheduling. Windows rename/delete additionally
// detects orphan statements keeping sqlite3_close_v2's zombie connection open.
func TestSQLiteCancellationReleasesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec("CREATE TABLE synthetic(value INTEGER); INSERT INTO synthetic VALUES(42)"); err != nil {
		t.Fatal(err)
	}
	var engineVersion string
	if err = db.QueryRow("SELECT sqlite_version()").Scan(&engineVersion); err != nil {
		t.Fatal(err)
	}
	canceled, completed := 0, 0
	for i := 0; i < 10000; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan struct{})
		go func(yields int) {
			for j := 0; j < yields; j++ {
				runtime.Gosched()
			}
			cancel()
			close(finished)
		}(1 + i%16)
		var value int
		err = db.QueryRowContext(ctx, "SELECT 42").Scan(&value)
		<-finished
		if err == nil {
			if value != 42 {
				t.Fatal("synthetic data changed")
			}
			completed++
			continue
		}
		var driverErr *sqlite.Error
		// SQLITE_INTERRUPT is the engine's public cancellation result. Some
		// timings expose it rather than context.Canceled; unrelated errors fail.
		interrupted := errors.As(err, &driverErr) && driverErr.Code()&0xff == 9 && ctx.Err() != nil
		if !errors.Is(err, context.Canceled) && !interrupted {
			t.Fatal("unexpected canceled-query result", err)
		}
		canceled++
	}
	if canceled == 0 {
		t.Fatal("cancellation stress never canceled a query")
	}
	// A normal uncanceled acquisition serializes behind all prior rows on this
	// one-connection pool. Do not replace cancellation with sleeps or unsafe
	// statement cleanup merely to make the file-release assertion pass.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var value int
	if err = connection.QueryRowContext(ctx, "SELECT value FROM synthetic").Scan(&value); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatal("data did not survive cancellation")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	released := path + ".released"
	if err = os.Rename(path, released); err != nil {
		t.Fatal("closed database still owns file handles", err)
	}
	reopened, err := sql.Open("sqlite", released)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.QueryRow("SELECT value FROM synthetic").Scan(&value); err != nil {
		reopened.Close()
		t.Fatal(err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatal("reopened data changed")
	}
	if err = os.Remove(released); err != nil {
		t.Fatal("reopened database did not release file", err)
	}
	t.Logf("sqlite=%s canceled=%d completed=%d file_release=verified", engineVersion, canceled, completed)
}
