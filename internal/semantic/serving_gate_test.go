package semantic

import "testing"

func TestServingGateModeOff(t *testing.T) {
	g := ServingGate{Mode: "off", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.9, 20, 10, false)
	if d.ServeSuperlinked || d.Reason != "mode_off" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestServingGateModeShadow(t *testing.T) {
	g := ServingGate{Mode: "shadow", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.9, 20, 10, false)
	if d.ServeSuperlinked || d.Reason != "shadow_only" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestServingGateLowConfidence(t *testing.T) {
	g := ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.2, 20, 10, false)
	if d.ServeSuperlinked || d.Reason != "low_confidence" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestServingGateHighLatency(t *testing.T) {
	g := ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.9, 200, 10, false)
	if d.ServeSuperlinked || d.Reason != "latency_exceeded" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestServingGateNoCandidates(t *testing.T) {
	g := ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.9, 20, 0, false)
	if d.ServeSuperlinked || d.Reason != "no_candidates" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestServingGateGatedHealthy(t *testing.T) {
	g := ServingGate{Mode: "gated", MinConfidence: 0.55, MaxLatencyMs: 120, MaxCandidates: 300}
	d := g.Decide(0.9, 20, 10, false)
	if !d.ServeSuperlinked || d.Reason != "served_superlinked" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}
