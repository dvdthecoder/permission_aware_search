package identifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTenantPatternPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "patterns.json")
	raw := `{
  "tenants": {
    "default": [
      { "name": "numeric_default", "type": "order_number", "regex": "\\b\\d{6,}\\b" }
    ],
    "tenant-z": [
      { "name": "tenant_z_pattern", "type": "order_number", "regex": "(?i)\\bZORD-\\d{6}\\b" }
    ]
  }
}`
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	reg := LoadPatternRegistry(cfgPath)
	out, matched := DetectWithRegistry("ZORD-123456", "tenant-z", reg)
	if len(out) == 0 {
		t.Fatalf("expected pattern match")
	}
	if matched != "tenant_z_pattern" {
		t.Fatalf("expected tenant pattern, got %q", matched)
	}
}
