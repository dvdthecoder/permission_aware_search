package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"permission_aware_search/internal/contracts"
	httpserver "permission_aware_search/internal/http"
	"permission_aware_search/internal/identifier"
	"permission_aware_search/internal/observability"
	"permission_aware_search/internal/policy"
	"permission_aware_search/internal/schema"
	"permission_aware_search/internal/search"
	"permission_aware_search/internal/semantic"
	sqlitestore "permission_aware_search/internal/store/sqlite"
)

func main() {
	loadDotEnv(".env", ".env.local")

	dbPath := envOrDefault("DB_PATH", "data/demo.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Wire schema registry first — single source of truth for all data-shape knowledge.
	// Must happen before migrations so seed functions can derive table names from it.
	reg := schema.New(schema.EcommerceDefinition())
	contracts.SetRegistry(reg)
	identifier.SetDefaultSchemaRegistry(reg)
	semantic.SetSchemaRegistry(reg)

	if err := runMigrations(db, reg); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	store := sqlitestore.NewAdapter(db, reg)
	policyEngine := policy.NewSQLiteEngine(db)
	searchService := search.NewService(store, policyEngine)

	framer := semantic.NewDeterministicIntentFramer()
	ruleAnalyzer := semantic.NewRuleSLMAnalyzerWithFramer(framer)
	ollamaEndpoint := envOrDefault("OLLAMA_ENDPOINT", "")
	intentRuleAnalyzer := ruleAnalyzer
	intentQwenAnalyzer := semantic.NewSLMLocalAnalyzerWithConfig(
		ollamaEndpoint,
		envOrDefault("OLLAMA_QWEN_MODEL", "qwen2.5:7b-instruct"),
		timeoutFromEnv("OLLAMA_QWEN_TIMEOUT_MS", 2400),
		framer,
	)
	intentLlamaAnalyzer := semantic.NewSLMLocalAnalyzerWithConfig(
		ollamaEndpoint,
		envOrDefault("OLLAMA_LLAMA_MODEL", envOrDefault("OLLAMA_MODEL", "llama3.1:8b-instruct")),
		timeoutFromEnv("OLLAMA_LLAMA_TIMEOUT_MS", intFromEnv("OLLAMA_TIMEOUT_MS", 2600)),
		framer,
	)
	rewriteIntentProviders := map[string]semantic.Analyzer{
		"intent-rule":         intentRuleAnalyzer,
		"intent-ollama-qwen":  intentQwenAnalyzer,
		"intent-ollama-llama": intentLlamaAnalyzer,
	}
	rewriteIntentAnalyzer := semantic.NewRoutedRewriteIntentAnalyzer(
		envOrDefault("REWRITE_INTENT_MODELS_PATH", "config/rewrite_intent_models.json"),
		rewriteIntentProviders,
		framer,
	)

	// Legacy provider alias retained for direct provider override compatibility.
	slmLocalAnalyzer := intentLlamaAnalyzer
	hashEmbed := semantic.NewHashDemoEmbeddingProvider(64)
	embeddingProvider := semantic.NewFallbackEmbeddingProvider(
		semantic.NewOllamaEmbeddingProvider(
			envOrDefault("OLLAMA_ENDPOINT", "http://localhost:11434"),
			envOrDefault("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
			timeoutFromEnv("OLLAMA_EMBED_TIMEOUT_MS", 1200),
		),
		hashEmbed,
	)
	semanticIndex := semantic.NewSQLiteEmbeddingIndexWithProvider(db, embeddingProvider)
	hybridRetriever := semantic.NewHybridCandidateRetriever(
		db,
		semanticIndex,
		envOrDefault("RETRIEVAL_MODELS_PATH", "config/retrieval_models.json"),
		embeddingProvider,
	)
	superlinkedMock := semantic.NewSuperlinkedMockAnalyzerWithFramerAndRetriever(
		framer,
		hybridRetriever,
		intFromEnv("RETRIEVAL_FUSION_CAP", intFromEnv("SUPERLINKED_TOPK", 100)),
	)
	superlinkedEndpoint := envOrDefault("SUPERLINKED_ENDPOINT", "http://localhost:8080/api/mock/superlinked")
	superlinkedAnalyzer := semantic.NewSuperlinkedAnalyzer(
		superlinkedEndpoint,
		timeoutFromEnv("SUPERLINKED_TIMEOUT_MS", 1500),
		rewriteIntentAnalyzer,
	)
	combinedAnalyzer := semantic.NewSLMSuperlinkedAnalyzer(rewriteIntentAnalyzer, superlinkedAnalyzer)
	semanticRouter := semantic.NewRouter(
		envOrDefault("SEMANTIC_PROVIDER", "slm-superlinked"),
		map[string]semantic.Analyzer{
			ruleAnalyzer.Name():          ruleAnalyzer,
			slmLocalAnalyzer.Name():      slmLocalAnalyzer,
			rewriteIntentAnalyzer.Name(): rewriteIntentAnalyzer,
			"intent-rule":                intentRuleAnalyzer,
			"intent-ollama-qwen":         intentQwenAnalyzer,
			"intent-ollama-llama":        intentLlamaAnalyzer,
			superlinkedMock.Name():       superlinkedMock,
			"superlinked":                superlinkedAnalyzer,
			combinedAnalyzer.Name():      combinedAnalyzer,
		},
	)
	metrics := observability.NewMetrics()
	server := httpserver.NewServer(searchService, metrics, db, semanticRouter)

	addr := envOrDefault("ADDR", ":8080")
	log.Printf("API listening on %s", addr)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func runMigrations(db *sql.DB, reg *schema.Registry) error {
	files := []string{
		"migrations/001_init.sql",
		"migrations/002_seed.sql",
		"migrations/003_seed_bulk.sql",
		"migrations/004_semantic_index.sql",
		"migrations/005_semantic_vectors.sql",
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		stmts := splitStatements(string(raw))
		for _, stmt := range stmts {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				errMsg := strings.ToLower(err.Error())
				if strings.Contains(errMsg, "duplicate column name") {
					continue
				}
				return fmt.Errorf("exec migration statement %q: %w", stmt, err)
			}
		}
	}
	if err := rebuildSemanticIndex(db, reg); err != nil {
		return fmt.Errorf("rebuild semantic index: %w", err)
	}
	return nil
}

func rebuildSemanticIndex(db *sql.DB, reg *schema.Registry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM semantic_index`); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM semantic_vectors`); err != nil {
		return err
	}
	if err = seedOrderSemanticIndex(tx, reg); err != nil {
		return err
	}
	if err = seedCustomerSemanticIndex(tx, reg); err != nil {
		return err
	}
	return tx.Commit()
}

func seedOrderSemanticIndex(tx *sql.Tx, reg *schema.Registry) error {
	// Table name comes from the schema registry; native column names are SQL-level
	// and correspond 1:1 to Field.NativeColumn values defined in ecommerce.go.
	orderTable, err := reg.GetTableName("order")
	if err != nil {
		return fmt.Errorf("schema registry: %w", err)
	}
	rows, err := tx.Query(`
SELECT
  tenant_id, id,
  coalesce(order_number, ''), coalesce(customer_email, ''), coalesce(order_state, ''),
  coalesce(shipment_state, ''), coalesce(payment_state, ''), coalesce(tracking_id, ''), coalesce(payment_reference, ''),
  coalesce(currency_code, ''), coalesce(cast(total_cent_amount as text), '')
FROM ` + orderTable)
	if err != nil {
		return err
	}
	defer rows.Close()

	stmt, err := tx.Prepare(`
INSERT INTO semantic_index (
  tenant_id, resource_type, resource_id, content_text, embedding_json,
  embedding_provider, embedding_model, embedding_dims, index_version, field_profile, updated_at
)
VALUES (?, 'order', ?, ?, ?, 'hash_demo', 'hash_demo_v1', 64, 'v2', 'order_tracking_text', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var tenantID, id, orderNumber, customerEmail, orderState, shipmentState, paymentState, trackingID, paymentRef, currencyCode, totalCentAmount string
		if err := rows.Scan(&tenantID, &id, &orderNumber, &customerEmail, &orderState, &shipmentState, &paymentState, &trackingID, &paymentRef, &currencyCode, &totalCentAmount); err != nil {
			return err
		}
		text := fmt.Sprintf(
			"order %s customer %s order_state %s shipment_state %s payment_state %s tracking %s payment_reference %s currency %s total_cent_amount %s",
			orderNumber,
			customerEmail,
			orderState,
			shipmentState,
			paymentState,
			trackingID,
			paymentRef,
			currencyCode,
			totalCentAmount,
		)
		emb := semantic.EmbedForIndex(text)
		if _, err := stmt.Exec(tenantID, id, text, emb); err != nil {
			return err
		}
		if _, err := tx.Exec(`
INSERT INTO semantic_vectors (tenant_id, resource_type, resource_id, field_profile, embedding_json, embedding_model, embedding_provider, index_version, updated_at)
VALUES (?, 'order', ?, 'order_tracking_text', ?, 'hash_demo_v1', 'hash_demo', 'v2', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`, tenantID, id, emb); err != nil {
			return err
		}
		returnsText := fmt.Sprintf(
			"order %s return_eligible %s return_status %s refund_status %s",
			orderNumber,
			"",
			"",
			"",
		)
		if _, err := tx.Exec(`
INSERT INTO semantic_vectors (tenant_id, resource_type, resource_id, field_profile, embedding_json, embedding_model, embedding_provider, index_version, updated_at)
VALUES (?, 'order', ?, 'order_returns_refunds_text', ?, 'hash_demo_v1', 'hash_demo', 'v2', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`, tenantID, id, semantic.EmbedForIndex(returnsText)); err != nil {
			return err
		}
		contextText := fmt.Sprintf(
			"order %s customer %s payment_reference %s total_cent_amount %s",
			orderNumber,
			customerEmail,
			paymentRef,
			totalCentAmount,
		)
		if _, err := tx.Exec(`
INSERT INTO semantic_vectors (tenant_id, resource_type, resource_id, field_profile, embedding_json, embedding_model, embedding_provider, index_version, updated_at)
VALUES (?, 'order', ?, 'order_customer_context_text', ?, 'hash_demo_v1', 'hash_demo', 'v2', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`, tenantID, id, semantic.EmbedForIndex(contextText)); err != nil {
			return err
		}
	}
	return rows.Err()
}

func seedCustomerSemanticIndex(tx *sql.Tx, reg *schema.Registry) error {
	// Table name from registry; column names are SQL-level NativeColumn values from ecommerce.go.
	customerTable, err := reg.GetTableName("customer")
	if err != nil {
		return fmt.Errorf("schema registry: %w", err)
	}
	rows, err := tx.Query(`
SELECT
  tenant_id, id,
  coalesce(customer_number, ''), coalesce(email, ''), coalesce(first_name, ''),
  coalesce(last_name, ''), coalesce(customer_group, ''), coalesce(vip_tier, '')
FROM ` + customerTable)
	if err != nil {
		return err
	}
	defer rows.Close()

	stmt, err := tx.Prepare(`
INSERT INTO semantic_index (
  tenant_id, resource_type, resource_id, content_text, embedding_json,
  embedding_provider, embedding_model, embedding_dims, index_version, field_profile, updated_at
)
VALUES (?, 'customer', ?, ?, ?, 'hash_demo', 'hash_demo_v1', 64, 'v2', 'customer_profile_text', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var tenantID, id, customerNumber, email, firstName, lastName, customerGroup, vipTier string
		if err := rows.Scan(&tenantID, &id, &customerNumber, &email, &firstName, &lastName, &customerGroup, &vipTier); err != nil {
			return err
		}
		text := fmt.Sprintf(
			"customer %s email %s first_name %s last_name %s customer_group %s vip_tier %s",
			customerNumber,
			email,
			firstName,
			lastName,
			customerGroup,
			vipTier,
		)
		emb := semantic.EmbedForIndex(text)
		if _, err := stmt.Exec(tenantID, id, text, emb); err != nil {
			return err
		}
		if _, err := tx.Exec(`
INSERT INTO semantic_vectors (tenant_id, resource_type, resource_id, field_profile, embedding_json, embedding_model, embedding_provider, index_version, updated_at)
VALUES (?, 'customer', ?, 'customer_profile_text', ?, 'hash_demo_v1', 'hash_demo', 'v2', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`, tenantID, id, emb); err != nil {
			return err
		}
		supportText := fmt.Sprintf(
			"customer %s email %s support_history normal vip_tier %s",
			customerNumber,
			email,
			vipTier,
		)
		if _, err := tx.Exec(`
INSERT INTO semantic_vectors (tenant_id, resource_type, resource_id, field_profile, embedding_json, embedding_model, embedding_provider, index_version, updated_at)
VALUES (?, 'customer', ?, 'customer_support_context_text', ?, 'hash_demo_v1', 'hash_demo', 'v2', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
`, tenantID, id, semantic.EmbedForIndex(supportText)); err != nil {
			return err
		}
	}
	return rows.Err()
}

func splitStatements(in string) []string {
	lines := strings.Split(in, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Split(strings.Join(filtered, "\n"), ";")
}

func envOrDefault(name, fallback string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return fallback
}

func timeoutFromEnv(name string, fallbackMs int) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(fallbackMs) * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return time.Duration(fallbackMs) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func intFromEnv(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

func loadDotEnv(paths ...string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			if key == "" {
				continue
			}
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			_ = os.Setenv(key, value)
		}
		_ = file.Close()
	}
}
