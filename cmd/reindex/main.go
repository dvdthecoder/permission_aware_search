package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := envOrDefault("DB_PATH", "data/demo.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	indexVersion := envOrDefault("REINDEX_VERSION", "v2")
	if _, err := db.Exec(`
INSERT OR REPLACE INTO semantic_vectors (
  tenant_id, resource_type, resource_id, field_profile, embedding_json,
  embedding_model, embedding_provider, index_version, updated_at
)
SELECT tenant_id, resource_type, resource_id, field_profile, embedding_json,
       embedding_model, embedding_provider, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM semantic_index
`, indexVersion); err != nil {
		log.Fatalf("reindex semantic_vectors failed: %v", err)
	}
	if _, err := db.Exec(`UPDATE semantic_index SET index_version = ?`, indexVersion); err != nil {
		log.Fatalf("stamp semantic_index version failed: %v", err)
	}
	log.Printf("reindex complete (indexVersion=%s)", indexVersion)
}

func envOrDefault(name, fallback string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return fallback
}
