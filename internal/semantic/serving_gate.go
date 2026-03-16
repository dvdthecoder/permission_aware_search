package semantic

import (
	"strings"
)

type GateDecision struct {
	ServeSuperlinked bool
	Reason           string
}

type ServingGate struct {
	Mode          string
	MinConfidence float64
	MaxLatencyMs  int64
	MaxCandidates int
}

func (g ServingGate) Decide(providerConfidence float64, providerLatencyMs int64, candidateCount int, clarificationNeeded bool) GateDecision {
	mode := strings.ToLower(strings.TrimSpace(g.Mode))
	if mode == "" {
		mode = "shadow"
	}
	if mode == "off" {
		return GateDecision{ServeSuperlinked: false, Reason: "mode_off"}
	}
	if mode == "shadow" {
		return GateDecision{ServeSuperlinked: false, Reason: "shadow_only"}
	}
	if clarificationNeeded {
		return GateDecision{ServeSuperlinked: false, Reason: "clarification_needed"}
	}
	if providerConfidence < g.MinConfidence {
		return GateDecision{ServeSuperlinked: false, Reason: "low_confidence"}
	}
	if providerLatencyMs > g.MaxLatencyMs {
		return GateDecision{ServeSuperlinked: false, Reason: "latency_exceeded"}
	}
	if candidateCount <= 0 {
		return GateDecision{ServeSuperlinked: false, Reason: "no_candidates"}
	}
	if g.MaxCandidates > 0 && candidateCount > g.MaxCandidates {
		return GateDecision{ServeSuperlinked: false, Reason: "too_many_candidates"}
	}
	return GateDecision{ServeSuperlinked: true, Reason: "served_superlinked"}
}
