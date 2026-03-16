package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"

	httpserver "permission_aware_search/internal/http"
	"permission_aware_search/internal/observability"
	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/search"
	"permission_aware_search/internal/semantic"
	sqlitestore "permission_aware_search/internal/store/sqlite"
)

func TestChatDebugIncludesLoopbackFields(t *testing.T) {
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
	defer func() { _ = os.Chdir(oldWd) }()

	if err := runMigrations(db, testReg); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	ds := sqlitestore.NewAdapter(db, testReg)
	pe := policy.NewSQLiteEngine(db)
	svc := search.NewService(ds, pe)
	metrics := observability.NewMetrics()

	slm := semantic.NewSLMLocalAnalyzer()
	supMock := semantic.NewSuperlinkedMockAnalyzer()
	combo := semantic.NewSLMSuperlinkedAnalyzer(slm, supMock)
	router := semantic.NewRouter("slm-superlinked", map[string]semantic.Analyzer{
		"slm-local":        slm,
		"superlinked-mock": supMock,
		"slm-superlinked":  combo,
	})

	server := httpserver.NewServer(svc, metrics, db, router)

	payload := map[string]interface{}{
		"message":         "show orders for the month",
		"provider":        "slm-superlinked",
		"contractVersion": "v2",
		"debug":           true,
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/query/interpret", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "alice")
	req.Header.Set("X-Tenant-Id", "tenant-a")
	req.Header.Set("X-Roles", "sales_rep")
	req.Header.Set("X-User-Attrs", `{"region":"west"}`)

	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	out := map[string]interface{}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	debugObj, ok := out["debug"].(map[string]interface{})
	if !ok {
		t.Fatalf("debug object missing")
	}
	rewrite, ok := debugObj["rewrite"].(map[string]interface{})
	if !ok {
		t.Fatalf("debug.rewrite missing")
	}
	for _, key := range []string{
		"slmRaw",
		"validationErrors",
		"repaired",
		"finalValidatedQuery",
		"rewriteIntentProvider",
		"rewriteIntentModelVersion",
		"rewriteIntentFallbackChain",
		"rewriteIntentGateReason",
	} {
		if _, exists := rewrite[key]; !exists {
			t.Fatalf("expected rewrite key %q", key)
		}
	}
	for _, key := range []string{
		"rewriteIntentProvider",
		"rewriteIntentModelVersion",
		"rewriteIntentFallbackChain",
		"rewriteIntentGateReason",
	} {
		if _, exists := out[key]; !exists {
			t.Fatalf("expected top-level key %q", key)
		}
	}
}
