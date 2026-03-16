package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/search"
	"permission_aware_search/internal/semantic"
	sqlitestore "permission_aware_search/internal/store/sqlite"
)

func TestGoldenPromptsAgainstSeededData(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))

	dbPath := filepath.Join(t.TempDir(), "demo.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to module root failed: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := runMigrations(db, testReg); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	ds := sqlitestore.NewAdapter(db, testReg)
	pe := policy.NewSQLiteEngine(db)
	svc := search.NewService(ds, pe)

	slm := semantic.NewSLMLocalAnalyzer()
	supMock := semantic.NewSuperlinkedMockAnalyzer()
	combo := semantic.NewSLMSuperlinkedAnalyzer(slm, supMock)

	subject := policy.Subject{
		UserID:   "mgr-a",
		TenantID: "tenant-a",
		Roles:    []string{"manager"},
		Attributes: map[string]string{
			"region": "west",
		},
	}

	prompts := []string{
		"Find order ORD-000001",
		"Find orders placed by customer with email customer00001@tenant-a.example.com",
		"What is the current status of order ORD-000001?",
		"Did payment succeed for order ORD-000002?",
		"Where is the shipment for order ORD-000001?",
		"Has the customer initiated a return for order ORD-000014?",
		"Show customer profile for CUST-000001",
		"Who placed order ORD-000001?",
		"Is order ORD-000001 associated with a high-risk customer?",
		"Give me a full investigation report for order ORD-000001",
	}

	for _, prompt := range prompts {
		res, err := combo.Analyze(context.Background(), semantic.AnalyzeRequest{Message: prompt, ContractVersion: "v2"})
		if err != nil {
			t.Fatalf("analyze failed for %q: %v", prompt, err)
		}
		if res.ClarificationNeeded {
			t.Fatalf("prompt unexpectedly ambiguous: %q", prompt)
		}
		if res.ResourceType == "" {
			t.Fatalf("missing resource type for %q", prompt)
		}

		sresp, err := svc.Search(context.Background(), subject, res.ResourceType, res.Query)
		if err != nil {
			t.Fatalf("search failed for %q: %v", prompt, err)
		}
		if sresp.AuthorizedCount == 0 {
			t.Fatalf("expected at least one authorized result for %q", prompt)
		}

		if strings.Contains(strings.ToLower(prompt), "order") {
			found := false
			for _, fs := range res.FilterSource {
				if fs.Field == "order.number" || fs.Field == "order.customer_email" || fs.Field == "order.customer_id" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected order-linked filter source in prompt %q", prompt)
			}
		}
	}
}
