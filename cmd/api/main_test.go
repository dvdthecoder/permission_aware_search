package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrationsSeedsMinimumCounts(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))

	dbPath := filepath.Join(t.TempDir(), "demo.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to module root failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := runMigrations(db, testReg); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM customers_docs WHERE tenant_id = 'tenant-a'", 20000)
	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM orders_docs WHERE tenant_id = 'tenant-a'", 20000)
	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM customers_docs WHERE tenant_id = 'tenant-b'", 500)
	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM orders_docs WHERE tenant_id = 'tenant-b'", 500)
	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM semantic_index WHERE tenant_id = 'tenant-a' AND resource_type = 'order'", 20000)
	assertCountAtLeast(t, db, "SELECT COUNT(1) FROM semantic_index WHERE tenant_id = 'tenant-a' AND resource_type = 'customer'", 20000)
}

func assertCountAtLeast(t *testing.T, db *sql.DB, query string, minimum int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count query failed (%s): %v", query, err)
	}
	if got < minimum {
		t.Fatalf("count below minimum for query %s: got %d want >= %d", query, got, minimum)
	}
}
