package search

import (
	"os"
	"testing"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/schema"
)

func TestMain(m *testing.M) {
	contracts.SetRegistry(schema.New(schema.EcommerceDefinition()))
	os.Exit(m.Run())
}
