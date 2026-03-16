package semantic

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

type RetrievalConfig struct {
	Mode          string           `json:"mode"`
	MinConfidence float64          `json:"minConfidence"`
	MaxLatencyMs  int64            `json:"maxLatencyMs"`
	TopKVector    int              `json:"topKVector"`
	TopKLexical   int              `json:"topKLexical"`
	FusionCap     int              `json:"fusionCap"`
	Weights       RetrievalWeights `json:"weights"`
}

type RetrievalWeights struct {
	Identifier float64 `json:"identifier"`
	Lexical    float64 `json:"lexical"`
	Vector     float64 `json:"vector"`
}

func defaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		Mode:          "hybrid_gated",
		MinConfidence: 0.50,
		MaxLatencyMs:  150,
		TopKVector:    120,
		TopKLexical:   120,
		FusionCap:     300,
		Weights: RetrievalWeights{
			Identifier: 0.20,
			Lexical:    0.35,
			Vector:     0.45,
		},
	}
}

func identifierHeavyWeights() RetrievalWeights {
	return RetrievalWeights{Identifier: 0.55, Lexical: 0.30, Vector: 0.15}
}

func loadRetrievalConfig(path string) RetrievalConfig {
	cfg := defaultRetrievalConfig()
	if strings.TrimSpace(path) != "" {
		if raw, err := os.ReadFile(path); err == nil {
			decoded := RetrievalConfig{}
			if err := json.Unmarshal(raw, &decoded); err == nil {
				mergeRetrievalConfig(&cfg, decoded)
			}
		}
	}
	cfg.Mode = strings.ToLower(strings.TrimSpace(envOrDefault("RETRIEVAL_MODE", cfg.Mode)))
	cfg.MinConfidence = retrievalFloatFromEnvOr("RETRIEVAL_MIN_CONFIDENCE", cfg.MinConfidence)
	cfg.MaxLatencyMs = int64(retrievalIntFromEnvOr("RETRIEVAL_MAX_LATENCY_MS", int(cfg.MaxLatencyMs)))
	cfg.TopKVector = retrievalIntFromEnvOr("RETRIEVAL_TOPK_VECTOR", cfg.TopKVector)
	cfg.TopKLexical = retrievalIntFromEnvOr("RETRIEVAL_TOPK_LEXICAL", cfg.TopKLexical)
	cfg.FusionCap = retrievalIntFromEnvOr("RETRIEVAL_FUSION_CAP", cfg.FusionCap)
	return cfg
}

func mergeRetrievalConfig(dst *RetrievalConfig, src RetrievalConfig) {
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.MinConfidence > 0 {
		dst.MinConfidence = src.MinConfidence
	}
	if src.MaxLatencyMs > 0 {
		dst.MaxLatencyMs = src.MaxLatencyMs
	}
	if src.TopKVector > 0 {
		dst.TopKVector = src.TopKVector
	}
	if src.TopKLexical > 0 {
		dst.TopKLexical = src.TopKLexical
	}
	if src.FusionCap > 0 {
		dst.FusionCap = src.FusionCap
	}
	if src.Weights.Identifier > 0 || src.Weights.Lexical > 0 || src.Weights.Vector > 0 {
		dst.Weights = src.Weights
	}
}

func retrievalIntFromEnvOr(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func retrievalFloatFromEnvOr(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
