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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	httpserver "permission_aware_search/internal/http"
	"permission_aware_search/internal/observability"
	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/search"
	"permission_aware_search/internal/semantic"
	sqlitestore "permission_aware_search/internal/store/sqlite"
)

func TestQueryInterpretSuperlinkedShadowCallsProviderButFallsBack(t *testing.T) {
	var calls int32
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidateIds":["ord-00001","ord-00002"],
			"scores":[0.92,0.87],
			"providerConfidence":0.93,
			"safeEvidence":["external_superlinked"],
			"providerLatencyMs":10
		}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"SUPERLINKED_MODE":           "shadow",
		"SUPERLINKED_MIN_CONFIDENCE": "0.55",
		"SUPERLINKED_MAX_LATENCY_MS": "120",
		"SUPERLINKED_TOPK":           "100",
		"SUPERLINKED_POST_MERGE_CAP": "300",
		"SUPERLINKED_TIMEOUT_MS":     "1000",
	})

	out := callQueryInterpret(t, api, "show open orders this week")
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatalf("expected external superlinked provider to be called in shadow mode")
	}
	notes := asStringSlice(out["semanticNotes"])
	if !contains(notes, "superlinked_gated_fallback:shadow_only") {
		t.Fatalf("expected shadow fallback note, got %v", notes)
	}
	if provider, _ := out["semanticProvider"].(string); provider != "slm-superlinked" {
		t.Fatalf("expected semanticProvider slm-superlinked, got %v", out["semanticProvider"])
	}
}

func TestQueryInterpretSuperlinkedGatedUsesRankOnlyForExplicitFilters(t *testing.T) {
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidateIds":["ord-00001","ord-00002","ord-00003"],
			"scores":[0.91,0.84,0.79],
			"providerConfidence":0.95,
			"safeEvidence":["external_superlinked"],
			"providerLatencyMs":8
		}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"SUPERLINKED_MODE":           "gated",
		"SUPERLINKED_MIN_CONFIDENCE": "0.55",
		"SUPERLINKED_MAX_LATENCY_MS": "120",
		"SUPERLINKED_TOPK":           "100",
		"SUPERLINKED_POST_MERGE_CAP": "300",
		"SUPERLINKED_TIMEOUT_MS":     "1000",
	})

	out := callQueryInterpret(t, api, "show open orders this week")
	notes := asStringSlice(out["semanticNotes"])
	if !contains(notes, "superlinked_served") {
		t.Fatalf("expected superlinked_served note, got %v", notes)
	}

	generated, ok := out["generatedQuery"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing generatedQuery")
	}
	rawFilters, ok := generated["filters"].([]interface{})
	if !ok {
		t.Fatalf("missing generatedQuery.filters")
	}
	foundIDIn := false
	for _, f := range rawFilters {
		m, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		field, _ := m["field"].(string)
		op, _ := m["op"].(string)
		if (field == "id" || field == "order.id") && strings.ToLower(op) == "in" {
			foundIDIn = true
			break
		}
	}
	if foundIDIn {
		t.Fatalf("did not expect id in filter for explicit support filters, got filters=%v", rawFilters)
	}
	if !contains(notes, "superlinked_rank_only_due_to_explicit_filters") {
		t.Fatalf("expected rank-only note, got %v", notes)
	}
}

func TestQueryInterpretPendingPaymentReturnsVisibleResults(t *testing.T) {
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidateIds":["ord-00001","ord-00002","ord-00003"],
			"scores":[0.91,0.84,0.79],
			"providerConfidence":0.95,
			"safeEvidence":["external_superlinked"],
			"providerLatencyMs":8
		}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"SUPERLINKED_MODE":           "gated",
		"SUPERLINKED_MIN_CONFIDENCE": "0.55",
		"SUPERLINKED_MAX_LATENCY_MS": "120",
		"SUPERLINKED_TOPK":           "100",
		"SUPERLINKED_POST_MERGE_CAP": "300",
		"SUPERLINKED_TIMEOUT_MS":     "1000",
	})

	out := callQueryInterpret(t, api, "orders with payment pending")
	notes := asStringSlice(out["semanticNotes"])
	if !contains(notes, "superlinked_rank_only_due_to_explicit_filters") {
		t.Fatalf("expected rank-only note, got %v", notes)
	}

	authorized, ok := out["authorizedCount"].(float64)
	if !ok {
		t.Fatalf("missing authorizedCount")
	}
	if authorized <= 0 {
		t.Fatalf("expected visible results for pending payment query, got output=%v", out)
	}
}

func newAPIServerForSuperlinkedIntegrationTest(t *testing.T, superlinkedEndpoint string, env map[string]string) http.Handler {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}

	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))

	dbPath := filepath.Join(t.TempDir(), "demo.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to module root failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := runMigrations(db, testReg); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	ds := sqlitestore.NewAdapter(db, testReg)
	pe := policy.NewSQLiteEngine(db)
	svc := search.NewService(ds, pe)
	metrics := observability.NewMetrics()

	slm := semantic.NewSLMLocalAnalyzer()
	superlinked := semantic.NewSuperlinkedAnalyzer(superlinkedEndpoint, time.Second, slm)
	combo := semantic.NewSLMSuperlinkedAnalyzer(slm, superlinked)
	router := semantic.NewRouter("slm-superlinked", map[string]semantic.Analyzer{
		"slm-local":       slm,
		"superlinked":     superlinked,
		"slm-superlinked": combo,
	})

	server := httpserver.NewServer(svc, metrics, db, router)
	return server.Routes()
}

func callQueryInterpret(t *testing.T, handler http.Handler, message string) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{
		"message":         message,
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
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", rec.Code, rec.Body.String())
	}

	out := map[string]interface{}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response unmarshal failed: %v", err)
	}
	return out
}

func asStringSlice(in interface{}) []string {
	raw, ok := in.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(arr []string, needle string) bool {
	for _, s := range arr {
		if s == needle {
			return true
		}
	}
	return false
}
