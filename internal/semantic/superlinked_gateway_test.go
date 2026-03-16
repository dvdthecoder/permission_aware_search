package semantic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func gatewayWithResponse(status int, body string) *HTTPSuperlinkedGateway {
	return &HTTPSuperlinkedGateway{
		endpoint: "http://mock",
		client: &http.Client{
			Timeout: time.Second,
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
		maxCandidates: 100,
	}
}

func TestHTTPSuperlinkedGatewayValidPayloadNormalizes(t *testing.T) {
	gw := gatewayWithResponse(http.StatusOK, `{
		"candidateIds":["ord-1","ord-2","ord-2"," "],
		"scores":[0.9,1.2,0.1,-1],
		"providerConfidence":0.8,
		"safeEvidence":["a","a","b"],
		"providerLatencyMs":10
	}`)

	out, err := gw.Analyze(context.Background(), GatewayRequest{
		Message: "where is order",
		TopK:    10,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if len(out.CandidateIDs) != 2 {
		t.Fatalf("expected 2 normalized candidate ids, got %d", len(out.CandidateIDs))
	}
	if out.Scores[1] != 1 {
		t.Fatalf("expected clamped score to 1.0, got %f", out.Scores[1])
	}
	if len(out.SafeEvidence) != 2 {
		t.Fatalf("expected deduped evidence, got %v", out.SafeEvidence)
	}
}

func TestHTTPSuperlinkedGatewayMalformedJSON(t *testing.T) {
	gw := gatewayWithResponse(http.StatusOK, `{"candidateIds":`)
	_, err := gw.Analyze(context.Background(), GatewayRequest{Message: "x"})
	if !errors.Is(err, ErrProviderDecode) {
		t.Fatalf("expected ErrProviderDecode, got %v", err)
	}
}

func TestHTTPSuperlinkedGatewayHTTP500(t *testing.T) {
	gw := gatewayWithResponse(http.StatusInternalServerError, `{"error":"boom"}`)
	_, err := gw.Analyze(context.Background(), GatewayRequest{Message: "x"})
	if !errors.Is(err, ErrProviderStatus) {
		t.Fatalf("expected ErrProviderStatus, got %v", err)
	}
}

func TestHTTPSuperlinkedGatewayEmptyCandidatesInvalid(t *testing.T) {
	gw := gatewayWithResponse(http.StatusOK, `{"candidateIds":[],"providerConfidence":0.7}`)
	_, err := gw.Analyze(context.Background(), GatewayRequest{Message: "x"})
	if !errors.Is(err, ErrInvalidProviderPayload) {
		t.Fatalf("expected ErrInvalidProviderPayload, got %v", err)
	}
}

func TestHTTPSuperlinkedGatewayOversizedCandidatesClipped(t *testing.T) {
	gw := gatewayWithResponse(http.StatusOK, `{
		"candidateIds":["ord-1","ord-2","ord-3","ord-4"],
		"scores":[0.1,0.2,0.3,0.4],
		"providerConfidence":0.6
	}`)
	gw.maxCandidates = 2

	out, err := gw.Analyze(context.Background(), GatewayRequest{Message: "x"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if len(out.CandidateIDs) != 2 {
		t.Fatalf("expected clipped candidate count 2, got %d", len(out.CandidateIDs))
	}
}

func TestHTTPSuperlinkedGatewayScoreLengthMismatchInvalid(t *testing.T) {
	gw := gatewayWithResponse(http.StatusOK, `{"candidateIds":["ord-1","ord-2"],"scores":[0.9],"providerConfidence":0.7}`)
	_, err := gw.Analyze(context.Background(), GatewayRequest{Message: "x"})
	if !errors.Is(err, ErrInvalidProviderPayload) {
		t.Fatalf("expected ErrInvalidProviderPayload, got %v", err)
	}
}
