package semantic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestHybridRetrieverIdentifierDominates(t *testing.T) {
	db := openHybridTestDB(t)
	insertOrder(t, db, "tenant-a", "ord-00001", "ORD-000001", "TRK-00000001", "PAY-00000001", "customer00001@tenant-a.example.com")
	insertOrder(t, db, "tenant-a", "ord-00002", "ORD-000002", "TRK-00000002", "PAY-00000002", "customer00002@tenant-a.example.com")
	insertSemantic(t, db, "tenant-a", "order", "ord-00001")
	insertSemantic(t, db, "tenant-a", "order", "ord-00002")

	index := NewSQLiteEmbeddingIndex(db)
	retriever := NewHybridCandidateRetriever(db, index, "", NewHashDemoEmbeddingProvider(64))
	res, err := retriever.Retrieve(context.Background(), RetrievalRequest{
		TenantID:     "tenant-a",
		ResourceType: "order",
		Message:      "ORD-000001",
		QueryShape:   "identifier_token",
		TopK:         10,
	})
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(res.CandidateIDs) == 0 || res.CandidateIDs[0] != "ord-00001" {
		t.Fatalf("expected ord-00001 top result, got %+v", res.CandidateIDs)
	}
	if res.Strategy == "" {
		t.Fatalf("expected retrieval strategy")
	}
}

func TestFallbackEmbeddingProviderUsesFallback(t *testing.T) {
	provider := NewFallbackEmbeddingProvider(failingEmbeddingProvider{}, NewHashDemoEmbeddingProvider(64))
	vecs, err := provider.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 64 {
		t.Fatalf("expected fallback embedding dimensions")
	}
}

type failingEmbeddingProvider struct{}

func (failingEmbeddingProvider) Name() string  { return "fail" }
func (failingEmbeddingProvider) Model() string { return "fail-model" }
func (failingEmbeddingProvider) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("boom")
}

func openHybridTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE orders_docs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			doc_json TEXT NOT NULL,
			order_number TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.orderNumber')) STORED,
			customer_id TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerId')) STORED,
			customer_email TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerEmail')) STORED,
			order_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.orderState')) STORED,
			shipment_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.shipmentState')) STORED,
			payment_state TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.paymentState')) STORED,
			tracking_id TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.shippingInfo.deliveries[0].parcels[0].trackingData.trackingId')) STORED,
			payment_reference TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.paymentInfo.paymentReference')) STORED
		)`,
		`CREATE TABLE customers_docs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			doc_json TEXT NOT NULL,
			customer_number TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerNumber')) STORED,
			email TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.email')) STORED,
			first_name TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.firstName')) STORED,
			last_name TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.lastName')) STORED,
			vip_tier TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.custom.fields.vipTier')) STORED,
			customer_group TEXT GENERATED ALWAYS AS (json_extract(doc_json, '$.customerGroup')) STORED
		)`,
		`CREATE TABLE semantic_index (
			tenant_id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			content_text TEXT NOT NULL,
			embedding_json TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			PRIMARY KEY (tenant_id, resource_type, resource_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec schema failed: %v", err)
		}
	}
	return db
}

func insertOrder(t *testing.T, db *sql.DB, tenant, id, orderNumber, trackingID, paymentRef, email string) {
	t.Helper()
	doc := map[string]interface{}{
		"id":            id,
		"orderNumber":   orderNumber,
		"customerId":    "cust-00001",
		"customerEmail": email,
		"orderState":    "Open",
		"shipmentState": "Ready",
		"paymentState":  "Pending",
		"paymentInfo": map[string]interface{}{
			"paymentReference": paymentRef,
		},
		"shippingInfo": map[string]interface{}{
			"deliveries": []interface{}{
				map[string]interface{}{
					"parcels": []interface{}{
						map[string]interface{}{
							"trackingData": map[string]interface{}{
								"trackingId": trackingID,
							},
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(doc)
	if _, err := db.Exec(`INSERT INTO orders_docs (id, tenant_id, doc_json) VALUES (?, ?, ?)`, id, tenant, string(raw)); err != nil {
		t.Fatalf("insert order failed: %v", err)
	}
}

func insertSemantic(t *testing.T, db *sql.DB, tenant, resourceType, id string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO semantic_index (tenant_id, resource_type, resource_id, content_text, embedding_json) VALUES (?, ?, ?, ?, ?)`,
		tenant, resourceType, id, "sample", EmbedForIndex("sample "+id),
	); err != nil {
		t.Fatalf("insert semantic index failed: %v", err)
	}
}
