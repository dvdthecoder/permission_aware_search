package observability

import (
	"sort"
	"sync"
	"time"
)

type Metrics struct {
	mu            sync.Mutex
	searchLatency []float64
	hiddenCounts  []int
}

func NewMetrics() *Metrics {
	return &Metrics{
		searchLatency: make([]float64, 0, 1024),
		hiddenCounts:  make([]int, 0, 1024),
	}
}

func (m *Metrics) RecordSearch(duration time.Duration, hiddenCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchLatency = append(m.searchLatency, float64(duration.Milliseconds()))
	m.hiddenCounts = append(m.hiddenCounts, hiddenCount)
}

func (m *Metrics) Snapshot() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]float64{}
	if len(m.searchLatency) == 0 {
		return out
	}
	vals := append([]float64(nil), m.searchLatency...)
	sort.Float64s(vals)
	out["p50_ms"] = percentile(vals, 50)
	out["p95_ms"] = percentile(vals, 95)
	out["p99_ms"] = percentile(vals, 99)

	hiddenTotal := 0
	for _, c := range m.hiddenCounts {
		hiddenTotal += c
	}
	out["hidden_avg"] = float64(hiddenTotal) / float64(len(m.hiddenCounts))
	return out
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}
