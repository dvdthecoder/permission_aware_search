package contracts

import (
	"os"
	"testing"

	"permission_aware_search/internal/schema"
)

func TestMain(m *testing.M) {
	SetRegistry(schema.New(schema.EcommerceDefinition()))
	os.Exit(m.Run())
}
