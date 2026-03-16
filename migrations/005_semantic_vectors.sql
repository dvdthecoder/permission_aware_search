ALTER TABLE semantic_index ADD COLUMN embedding_provider TEXT NOT NULL DEFAULT 'hash_demo';
ALTER TABLE semantic_index ADD COLUMN embedding_model TEXT NOT NULL DEFAULT 'hash_demo_v1';
ALTER TABLE semantic_index ADD COLUMN embedding_dims INTEGER NOT NULL DEFAULT 64;
ALTER TABLE semantic_index ADD COLUMN index_version TEXT NOT NULL DEFAULT 'v1';
ALTER TABLE semantic_index ADD COLUMN field_profile TEXT NOT NULL DEFAULT 'default_profile';

CREATE TABLE IF NOT EXISTS semantic_vectors (
  tenant_id TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  field_profile TEXT NOT NULL,
  embedding_json TEXT NOT NULL,
  embedding_model TEXT NOT NULL,
  embedding_provider TEXT NOT NULL,
  index_version TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  PRIMARY KEY (tenant_id, resource_type, resource_id, field_profile)
);

CREATE INDEX IF NOT EXISTS idx_semantic_vectors_lookup
ON semantic_vectors(tenant_id, resource_type, field_profile);
