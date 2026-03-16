package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"permission_aware_search/internal/contracts"
)

type RewriteIntentProviderConfig struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Model         string  `json:"model"`
	Endpoint      string  `json:"endpoint"`
	TimeoutMs     int     `json:"timeoutMs"`
	MinConfidence float64 `json:"minConfidence"`
}

type RewriteIntentRouteConfig struct {
	TenantID            string   `json:"tenant,omitempty"`
	QueryShape          string   `json:"queryShape,omitempty"`
	IntentCategory      string   `json:"intentCategory,omitempty"`
	PrimaryProviderID   string   `json:"primaryProviderId"`
	FallbackProviderIDs []string `json:"fallbackProviderIds,omitempty"`
}

type RewriteIntentDefaultConfig struct {
	PrimaryProviderID   string   `json:"primaryProviderId"`
	FallbackProviderIDs []string `json:"fallbackProviderIds,omitempty"`
}

type RewriteIntentModelConfig struct {
	Providers []RewriteIntentProviderConfig `json:"providers"`
	Routes    []RewriteIntentRouteConfig    `json:"routes,omitempty"`
	Defaults  RewriteIntentDefaultConfig    `json:"defaults"`
}

type RoutedRewriteIntentAnalyzer struct {
	config    RewriteIntentModelConfig
	providers map[string]Analyzer
	framer    IntentFramer
}

func NewRoutedRewriteIntentAnalyzer(configPath string, providers map[string]Analyzer, framer IntentFramer) *RoutedRewriteIntentAnalyzer {
	if framer == nil {
		framer = NewDeterministicIntentFramer()
	}
	cfg := loadRewriteIntentModelConfig(configPath)
	return &RoutedRewriteIntentAnalyzer{
		config:    cfg,
		providers: providers,
		framer:    framer,
	}
}

func (a *RoutedRewriteIntentAnalyzer) Name() string { return "rewrite-intent-routed" }

func (a *RoutedRewriteIntentAnalyzer) Analyze(ctx context.Context, req AnalyzeRequest) (AnalyzeResult, error) {
	if req.ContractVersion == "" {
		req.ContractVersion = contracts.ContractVersionV2
	}
	if a.framer == nil {
		a.framer = NewDeterministicIntentFramer()
	}
	hint := a.framer.Frame(req.Message, req.ContractVersion, req.ResourceHint)
	intentCategoryHint := hint.IntentCategory
	chain := a.resolveProviderChain(req.TenantID, req.QueryShape, intentCategoryHint)

	attempted := make([]string, 0, len(chain))
	gateReasons := make([]string, 0, len(chain))
	for _, providerID := range chain {
		provider, ok := a.providers[providerID]
		if !ok || provider == nil {
			attempted = append(attempted, providerID)
			gateReasons = append(gateReasons, providerID+":provider_not_found")
			continue
		}
		attempted = append(attempted, providerID)

		cfg := a.providerConfig(providerID)
		result, err := a.callProviderWithTimeout(ctx, provider, req, cfg.TimeoutMs)
		if err != nil {
			gateReasons = append(gateReasons, providerID+":timeout_or_error")
			continue
		}

		normalized, errs := validateRemoteResult(result, req.ContractVersion)
		if len(errs) > 0 {
			gateReasons = append(gateReasons, providerID+":invalid_output")
			continue
		}
		if cfg.MinConfidence > 0 && normalized.Confidence < cfg.MinConfidence {
			gateReasons = append(gateReasons, providerID+":low_confidence")
			continue
		}

		normalized.RewriteIntentProvider = providerID
		if cfg.Model != "" {
			normalized.RewriteIntentModelVersion = cfg.Model
		} else {
			normalized.RewriteIntentModelVersion = provider.Name()
		}
		normalized.RewriteIntentFallbackChain = append([]string{}, attempted...)
		if len(gateReasons) > 0 {
			normalized.RewriteIntentGateReason = strings.Join(gateReasons, ";")
		}
		normalized.Notes = appendUnique(normalized.Notes, "rewrite_intent_provider:"+providerID)
		return normalized, nil
	}

	ruleProvider := a.providers["intent-rule"]
	if ruleProvider == nil {
		return AnalyzeResult{}, fmt.Errorf("no rewrite/intent provider available")
	}
	fallbackRes, err := ruleProvider.Analyze(ctx, req)
	if err != nil {
		return AnalyzeResult{}, err
	}
	fallbackRes.RewriteIntentProvider = "intent-rule"
	fallbackRes.RewriteIntentModelVersion = "deterministic-rule"
	fallbackRes.RewriteIntentFallbackChain = append(attempted, "intent-rule")
	fallbackRes.RewriteIntentGateReason = strings.Join(gateReasons, ";")
	fallbackRes.Notes = appendUnique(fallbackRes.Notes, "rewrite_intent_router_fallback:intent-rule")
	return fallbackRes, nil
}

func (a *RoutedRewriteIntentAnalyzer) callProviderWithTimeout(ctx context.Context, provider Analyzer, req AnalyzeRequest, timeoutMs int) (AnalyzeResult, error) {
	timeout := 1500 * time.Millisecond
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return provider.Analyze(cctx, req)
}

func (a *RoutedRewriteIntentAnalyzer) resolveProviderChain(tenantID, queryShape, intentCategory string) []string {
	queryShape = strings.TrimSpace(queryShape)
	intentCategory = strings.TrimSpace(intentCategory)
	tenantID = strings.TrimSpace(tenantID)

	for _, route := range a.config.Routes {
		if route.TenantID != "" && route.TenantID != tenantID {
			continue
		}
		if route.QueryShape != "" && route.QueryShape != queryShape {
			continue
		}
		if route.IntentCategory != "" && route.IntentCategory != intentCategory {
			continue
		}
		chain := append([]string{}, route.PrimaryProviderID)
		chain = append(chain, route.FallbackProviderIDs...)
		return ensureRuleFallback(chain)
	}

	chain := append([]string{}, a.config.Defaults.PrimaryProviderID)
	chain = append(chain, a.config.Defaults.FallbackProviderIDs...)
	return ensureRuleFallback(chain)
}

func (a *RoutedRewriteIntentAnalyzer) providerConfig(providerID string) RewriteIntentProviderConfig {
	for _, p := range a.config.Providers {
		if p.ID == providerID {
			return p
		}
	}
	return RewriteIntentProviderConfig{ID: providerID, TimeoutMs: 1500, MinConfidence: 0.0}
}

func ensureRuleFallback(chain []string) []string {
	out := make([]string, 0, len(chain)+1)
	seen := map[string]struct{}{}
	for _, c := range chain {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if _, ok := seen["intent-rule"]; !ok {
		out = append(out, "intent-rule")
	}
	return out
}

func loadRewriteIntentModelConfig(path string) RewriteIntentModelConfig {
	def := RewriteIntentModelConfig{
		Providers: []RewriteIntentProviderConfig{
			{ID: "intent-rule", Kind: "deterministic_rule", Model: "deterministic-rule", TimeoutMs: 200, MinConfidence: 0.0},
			{ID: "intent-ollama-qwen", Kind: "ollama_local", Model: "qwen2.5:7b-instruct", Endpoint: envOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"), TimeoutMs: 2400, MinConfidence: 0.55},
			{ID: "intent-ollama-llama", Kind: "ollama_local", Model: "llama3.1:8b-instruct", Endpoint: envOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"), TimeoutMs: 2600, MinConfidence: 0.50},
		},
		Routes: []RewriteIntentRouteConfig{
			{QueryShape: "identifier_token", PrimaryProviderID: "intent-rule"},
			{QueryShape: "contact_lookup", PrimaryProviderID: "intent-rule"},
			{QueryShape: "typeahead_prefix", PrimaryProviderID: "intent-rule"},
			{QueryShape: "unsupported_domain", PrimaryProviderID: "intent-rule"},
			{QueryShape: "sentence_nl", PrimaryProviderID: "intent-ollama-qwen", FallbackProviderIDs: []string{"intent-ollama-llama"}},
			{QueryShape: "keyword_phrase", PrimaryProviderID: "intent-ollama-qwen", FallbackProviderIDs: []string{"intent-ollama-llama"}},
		},
		Defaults: RewriteIntentDefaultConfig{
			PrimaryProviderID:   "intent-ollama-qwen",
			FallbackProviderIDs: []string{"intent-ollama-llama"},
		},
	}
	if strings.TrimSpace(path) == "" {
		return def
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	cfg := RewriteIntentModelConfig{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return def
	}
	if cfg.Defaults.PrimaryProviderID == "" {
		cfg.Defaults = def.Defaults
	}
	if override := strings.TrimSpace(os.Getenv("REWRITE_INTENT_PROVIDER_DEFAULT")); override != "" {
		cfg.Defaults.PrimaryProviderID = override
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = def.Providers
	}
	cfg.Defaults.FallbackProviderIDs = append([]string{}, cfg.Defaults.FallbackProviderIDs...)
	return cfg
}
