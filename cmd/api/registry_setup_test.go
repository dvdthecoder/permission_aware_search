package main

import (
	"os"
	"testing"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/identifier"
	"permission_aware_search/internal/schema"
	"permission_aware_search/internal/semantic"
)

// testReg is the shared schema registry for all integration tests in this package.
var testReg *schema.Registry

func TestMain(m *testing.M) {
	testReg = schema.New(schema.EcommerceDefinition())
	contracts.SetRegistry(testReg)
	identifier.SetDefaultSchemaRegistry(testReg)
	semantic.SetSchemaRegistry(testReg)
	os.Exit(m.Run())
}
