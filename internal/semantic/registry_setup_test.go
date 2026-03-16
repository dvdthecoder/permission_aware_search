package semantic

import (
	"os"
	"testing"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/schema"
)

func TestMain(m *testing.M) {
	reg := schema.New(schema.EcommerceDefinition())
	contracts.SetRegistry(reg)
	SetSchemaRegistry(reg)
	os.Exit(m.Run())
}
