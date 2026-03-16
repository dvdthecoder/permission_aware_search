package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"permission_aware_search/internal/store"
)

type countingAnalyzer struct {
	name  string
	calls *int32
	fn    func(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error)
}

func (a countingAnalyzer) Name() string { return a.name }
func (a countingAnalyzer) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if a.calls != nil {
		atomic.AddInt32(a.calls, 1)
	}
	return a.fn(ctx, req)
}

func TestRewriteIntentRouterSelectsProviderByQueryShape(t *testing.T) {
	cfg := writeRewriteRouterConfig(t, RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", TimeoutMs: 200, MinConfidence: 0},
			{ID: "intent-ollama-qwen", TimeoutMs: 300, MinConfidence: 0.5},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen"},
		},
		Defaults: RewriteIntentDefaultConfig{PrimaryProviderID: "intent-rule"},
	})

	router := NewRoutedRewriteIntentAnalyzer(cfg, map[string]Analyzer{
		"intent-rule":        staticAnalyzer{name: "rule", res: validAnalyzeResult(0.7)},
		"intent-ollama-qwen": staticAnalyzer{name: "qwen", res: validAnalyzeResult(0.9)},
	}, NewDeterministicIntentFramer())

	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show open orders this week",
		ContractVersion: "v2",
		QueryShape:      "sentence_nl",
		TenantID:        "tenant-a",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.RewriteIntentProvider != "intent-ollama-qwen" {
		t.Fatalf("expected qwen provider, got %s", res.RewriteIntentProvider)
	}
	if got := strings.Join(res.RewriteIntentFallbackChain, ","); got != "intent-ollama-qwen" {
		t.Fatalf("unexpected fallback chain: %s", got)
	}
}

func TestRewriteIntentRouterFallsBackOnLowConfidence(t *testing.T) {
	cfg := writeRewriteRouterConfig(t, RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", TimeoutMs: 100, MinConfidence: 0},
			{ID: "intent-ollama-qwen", TimeoutMs: 300, MinConfidence: 0.8},
			{ID: "intent-ollama-llama", TimeoutMs: 300, MinConfidence: 0.5},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen", FallbackProviderIDs: []string{"intent-ollama-llama"}},
		},
		Defaults: RewriteIntentDefaultConfig{PrimaryProviderID: "intent-rule"},
	})

	router := NewRoutedRewriteIntentAnalyzer(cfg, map[string]Analyzer{
		"intent-rule":         staticAnalyzer{name: "rule", res: validAnalyzeResult(0.6)},
		"intent-ollama-qwen":  staticAnalyzer{name: "qwen", res: validAnalyzeResult(0.2)},
		"intent-ollama-llama": staticAnalyzer{name: "llama", res: validAnalyzeResult(0.9)},
	}, NewDeterministicIntentFramer())

	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show orders for the month",
		ContractVersion: "v2",
		QueryShape:      "sentence_nl",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.RewriteIntentProvider != "intent-ollama-llama" {
		t.Fatalf("expected llama fallback, got %s", res.RewriteIntentProvider)
	}
	if !strings.Contains(res.RewriteIntentGateReason, "low_confidence") {
		t.Fatalf("expected low_confidence gate reason, got %s", res.RewriteIntentGateReason)
	}
}

func TestRewriteIntentRouterFallsBackOnTimeout(t *testing.T) {
	cfg := writeRewriteRouterConfig(t, RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", TimeoutMs: 100, MinConfidence: 0},
			{ID: "intent-ollama-qwen", TimeoutMs: 10, MinConfidence: 0.5},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen"},
		},
		Defaults: RewriteIntentDefaultConfig{PrimaryProviderID: "intent-rule"},
	})

	router := NewRoutedRewriteIntentAnalyzer(cfg, map[string]Analyzer{
		"intent-rule": staticAnalyzer{name: "rule", res: validAnalyzeResult(0.8)},
		"intent-ollama-qwen": countingAnalyzer{
			name: "qwen",
			fn: func(ctx context.Context, _ AnalyzeRequest) (AnalyzeResult, error) {
				<-ctx.Done()
				return AnalyzeResult{}, ctx.Err()
			},
		},
	}, NewDeterministicIntentFramer())

	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show open orders this week",
		ContractVersion: "v2",
		QueryShape:      "sentence_nl",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.RewriteIntentProvider != "intent-rule" {
		t.Fatalf("expected deterministic fallback, got %s", res.RewriteIntentProvider)
	}
	if !strings.Contains(res.RewriteIntentGateReason, "timeout_or_error") {
		t.Fatalf("expected timeout gate reason, got %s", res.RewriteIntentGateReason)
	}
}

func TestRewriteIntentRouterDeterministicForIdentifierShape(t *testing.T) {
	cfg := writeRewriteRouterConfig(t, RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", TimeoutMs: 100, MinConfidence: 0},
			{ID: "intent-ollama-qwen", TimeoutMs: 100, MinConfidence: 0.2},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "identifier_token", PrimaryProviderID: "intent-rule"},
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen"},
		},
		Defaults: RewriteIntentDefaultConfig{PrimaryProviderID: "intent-ollama-qwen"},
	})

	var ruleCalls int32
	var qwenCalls int32
	router := NewRoutedRewriteIntentAnalyzer(cfg, map[string]Analyzer{
		"intent-rule": countingAnalyzer{
			name:  "rule",
			calls: &ruleCalls,
			fn: func(_ context.Context, _ AnalyzeRequest) (AnalyzeResult, error) {
				return validAnalyzeResult(0.9), nil
			},
		},
		"intent-ollama-qwen": countingAnalyzer{
			name:  "qwen",
			calls: &qwenCalls,
			fn: func(_ context.Context, _ AnalyzeRequest) (AnalyzeResult, error) {
				return AnalyzeResult{}, errors.New("should not be called")
			},
		},
	}, NewDeterministicIntentFramer())

	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:         "ORD-000001",
		ContractVersion: "v2",
		QueryShape:      "identifier_token",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.RewriteIntentProvider != "intent-rule" {
		t.Fatalf("expected intent-rule provider, got %s", res.RewriteIntentProvider)
	}
	if atomic.LoadInt32(&ruleCalls) != 1 {
		t.Fatalf("expected rule provider call count=1, got %d", ruleCalls)
	}
	if atomic.LoadInt32(&qwenCalls) != 0 {
		t.Fatalf("expected qwen not called, got %d", qwenCalls)
	}
}

func TestRewriteIntentRouterFallsBackOnInvalidProviderSchema(t *testing.T) {
	cfg := writeRewriteRouterConfig(t, RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", TimeoutMs: 100, MinConfidence: 0},
			{ID: "intent-ollama-qwen", TimeoutMs: 100, MinConfidence: 0.1},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen"},
		},
		Defaults: RewriteIntentDefaultConfig{PrimaryProviderID: "intent-rule"},
	})

	router := NewRoutedRewriteIntentAnalyzer(cfg, map[string]Analyzer{
		"intent-rule": staticAnalyzer{name: "rule", res: validAnalyzeResult(0.8)},
		"intent-ollama-qwen": staticAnalyzer{
			name: "qwen",
			res: AnalyzeResult{
				Intent:         "search_order",
				IntentCategory: "wismo",
				ResourceType:   "order",
				Confidence:     0.9,
				Query: store.QueryDSL{
					ContractVersion: "v2",
					IntentCategory:  "wismo",
					Filters:         []store.Filter{{Field: "drop_table", Op: "hack", Value: "x"}},
					Sort:            store.Sort{Field: "order.created_at", Dir: "desc"},
					Page:            store.Page{Limit: 20},
				},
			},
		},
	}, NewDeterministicIntentFramer())

	res, err := router.Analyze(context.Background(), AnalyzeRequest{
		Message:         "show open orders this week",
		ContractVersion: "v2",
		QueryShape:      "sentence_nl",
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if res.RewriteIntentProvider != "intent-rule" {
		t.Fatalf("expected intent-rule fallback, got %s", res.RewriteIntentProvider)
	}
	if !strings.Contains(res.RewriteIntentGateReason, "invalid_output") {
		t.Fatalf("expected invalid_output gate reason, got %s", res.RewriteIntentGateReason)
	}
}

func validAnalyzeResult(conf float64) AnalyzeResult {
	return AnalyzeResult{
		Intent:         "search_order",
		IntentCategory: "wismo",
		ResourceType:   "order",
		Confidence:     conf,
		Query: store.QueryDSL{
			ContractVersion: "v2",
			IntentCategory:  "wismo",
			Filters:         []store.Filter{{Field: "order.created_at", Op: "gte", Value: "2025-01-01T00:00:00Z"}},
			Sort:            store.Sort{Field: "order.created_at", Dir: "desc"},
			Page:            store.Page{Limit: 20},
		},
	}
}

func writeRewriteRouterConfig(t *testing.T, cfg RewriteIntentModelConfig) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rewrite_intent_models.json")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config failed: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	// Small sleep avoids flaky fs timestamp races on some environments when reading immediately.
	time.Sleep(5 * time.Millisecond)
	return path
}
