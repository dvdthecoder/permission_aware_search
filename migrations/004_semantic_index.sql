CREATE TABLE IF NOT EXISTS semantic_index (
  tenant_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  content_text TEXT NOT NULL,
  embedding_json TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  PRIMARY KEY (tenant_id, resource_type, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_semantic_index_lookup
ON semantic_index(tenant_id, resource_type);
