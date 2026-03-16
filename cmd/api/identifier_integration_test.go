package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryInterpretIdentifierFastPathOrderNumber(t *testing.T) {
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"candidateIds":["ord-00001"],"scores":[0.9],"providerConfidence":0.9}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"IDENTIFIER_FAST_PATH_ENABLED":        "true",
		"IDENTIFIER_GROUPED_RESPONSE_ENABLED": "true",
	})
	out := callQueryInterpret(t, api, "ORD-000001")
	if got, _ := out["resolutionMode"].(string); got != "identifier_fast_path" {
		t.Fatalf("expected identifier_fast_path resolution mode, got %v", out["resolutionMode"])
	}
	if _, ok := out["groupedMatches"].([]interface{}); !ok {
		t.Fatalf("expected groupedMatches in response")
	}
}

func TestQueryInterpretIdentifierFastPathPaymentReference(t *testing.T) {
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"candidateIds":["ord-00001"],"scores":[0.9],"providerConfidence":0.9}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"IDENTIFIER_FAST_PATH_ENABLED":        "true",
		"IDENTIFIER_GROUPED_RESPONSE_ENABLED": "true",
	})
	out := callQueryInterpret(t, api, "PAY-00001001")
	if got, _ := out["resolutionMode"].(string); got != "identifier_fast_path" {
		t.Fatalf("expected identifier_fast_path resolution mode, got %v", out["resolutionMode"])
	}
	rawGroups, ok := out["groupedMatches"].([]interface{})
	if !ok || len(rawGroups) == 0 {
		t.Fatalf("expected groupedMatches in response")
	}
	hasPaymentGroup := false
	for _, g := range rawGroups {
		m, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		if field, _ := m["matchField"].(string); strings.EqualFold(field, "payment.reference") {
			hasPaymentGroup = true
			break
		}
	}
	if !hasPaymentGroup {
		t.Fatalf("expected payment.reference group in groupedMatches")
	}
}

func TestQueryInterpretNoOpShortQuery(t *testing.T) {
	superSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidateIds":["ord-00001"],"scores":[0.9],"providerConfidence":0.9}`))
	}))
	defer superSrv.Close()

	api := newAPIServerForSuperlinkedIntegrationTest(t, superSrv.URL, map[string]string{
		"IDENTIFIER_FAST_PATH_ENABLED":        "true",
		"IDENTIFIER_GROUPED_RESPONSE_ENABLED": "true",
	})
	out := callQueryInterpret(t, api, "x")
	if got, _ := out["pathTaken"].(string); got != "no_op_short_query" {
		t.Fatalf("expected no_op_short_query path, got %v", out["pathTaken"])
	}
	if got, _ := out["queryShape"].(string); got != "typeahead_prefix" {
		t.Fatalf("expected typeahead_prefix shape, got %v", out["queryShape"])
	}
}
