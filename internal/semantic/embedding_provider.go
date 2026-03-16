package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type EmbeddingProvider interface {
	Name() string
	Model() string
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type HashDemoEmbeddingProvider struct {
	dims int
}

func NewHashDemoEmbeddingProvider(dims int) *HashDemoEmbeddingProvider {
	if dims <= 0 {
		dims = defaultEmbeddingDims
	}
	return &HashDemoEmbeddingProvider{dims: dims}
}

func (p *HashDemoEmbeddingProvider) Name() string  { return "hash_demo" }
func (p *HashDemoEmbeddingProvider) Model() string { return "hash_demo_v1" }

func (p *HashDemoEmbeddingProvider) Embed(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, t := range texts {
		out = append(out, embedText(t, p.dims))
	}
	return out, nil
}

type OllamaEmbeddingProvider struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaEmbeddingProvider(endpoint, model string, timeout time.Duration) *OllamaEmbeddingProvider {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "http://localhost:11434"
	}
	if strings.TrimSpace(model) == "" {
		model = "nomic-embed-text"
	}
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}
	return &OllamaEmbeddingProvider{
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		client:   &http.Client{Timeout: timeout},
	}
}

func (p *OllamaEmbeddingProvider) Name() string  { return "ollama_embed" }
func (p *OllamaEmbeddingProvider) Model() string { return p.model }

func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, t := range texts {
		vec, err := p.embedOne(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	return out, nil
}

func (p *OllamaEmbeddingProvider) embedOne(ctx context.Context, text string) ([]float64, error) {
	body := map[string]interface{}{
		"model":  p.model,
		"prompt": text,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/embeddings", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embeddings status=%d", resp.StatusCode)
	}
	decoded := struct {
		Embedding []float64 `json:"embedding"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding")
	}
	return decoded.Embedding, nil
}

type FallbackEmbeddingProvider struct {
	primary  EmbeddingProvider
	fallback EmbeddingProvider
}

func NewFallbackEmbeddingProvider(primary, fallback EmbeddingProvider) *FallbackEmbeddingProvider {
	return &FallbackEmbeddingProvider{primary: primary, fallback: fallback}
}

func (p *FallbackEmbeddingProvider) Name() string {
	if p.primary == nil {
		if p.fallback == nil {
			return "unknown"
		}
		return p.fallback.Name()
	}
	return p.primary.Name()
}

func (p *FallbackEmbeddingProvider) Model() string {
	if p.primary == nil {
		if p.fallback == nil {
			return "unknown"
		}
		return p.fallback.Model()
	}
	return p.primary.Model()
}

func (p *FallbackEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if p.primary != nil {
		if vecs, err := p.primary.Embed(ctx, texts); err == nil && len(vecs) == len(texts) {
			return vecs, nil
		}
	}
	if p.fallback == nil {
		return nil, fmt.Errorf("no embedding provider available")
	}
	return p.fallback.Embed(ctx, texts)
}
