package semantic

import (
	"fmt"
	"strings"

	"permission_aware_search/internal/contracts"
	"permission_aware_search/internal/schema"
)

// SchemaProvider provides field schema information for query rewriting.
type SchemaProvider interface {
	GetSchema(resourceType, contractVersion string) *ResourceSchema
}

// ResourceSchema defines the structure and constraints for a resource type.
type ResourceSchema struct {
	ResourceType string
	Fields       []FieldDef
}

// FieldDef defines a single field with its type, allowed operators, and intent scopes.
type FieldDef struct {
	Name         string
	Type         string   // string, int, float, timestamp, enum
	Description  string
	EnumValues   []string // For enum types
	Operators    []string // Allowed operators for this field
	IntentScopes []string // Which intents can access this field
	Example      string   // Example value
}

// RenderFieldList returns a markdown table of all fields with their properties.
func (rs *ResourceSchema) RenderFieldList() string {
	var b strings.Builder
	b.WriteString("| Field Name | Type | Allowed Operators | Intent Scope | Description | Example |\n")
	b.WriteString("|------------|------|-------------------|--------------|-------------|---------|")
	for _, f := range rs.Fields {
		b.WriteString(fmt.Sprintf("\n| %s | %s | %s | %s | %s | %s |",
			f.Name,
			f.Type,
			strings.Join(f.Operators, ", "),
			strings.Join(f.IntentScopes, ", "),
			f.Description,
			f.Example,
		))
	}
	return b.String()
}

// ContractV2SchemaProvider provides schema information derived from the schema registry.
// All field names, enum values, operators, and intent scopes come from
// schema.EcommerceDefinition() via the registry — no values are hardcoded here.
type ContractV2SchemaProvider struct {
	reg *schema.Registry
}

// NewContractV2SchemaProvider creates a schema provider backed by the registry.
func NewContractV2SchemaProvider(reg *schema.Registry) *ContractV2SchemaProvider {
	return &ContractV2SchemaProvider{reg: reg}
}

func (p *ContractV2SchemaProvider) GetSchema(resourceType, contractVersion string) *ResourceSchema {
	res, ok := p.reg.GetResource(resourceType)
	if !ok {
		return &ResourceSchema{ResourceType: resourceType, Fields: []FieldDef{}}
	}

	fields := make([]FieldDef, 0, len(res.Fields))
	for _, f := range res.Fields {
		fd := FieldDef{
			Name:        f.Name,
			Type:        f.Type,
			Description: f.Description,
			EnumValues:  f.EnumValues,
			Operators:   f.Operators,
			Example:     f.Example,
		}
		if contractVersion == contracts.ContractVersionV1 {
			// V1: all fields accessible, no intent scoping
			fd.IntentScopes = []string{contracts.IntentDefault}
		} else {
			fd.IntentScopes = f.IntentScopes
		}
		fields = append(fields, fd)
	}
	return &ResourceSchema{ResourceType: resourceType, Fields: fields}
}

// GetDefaultSchemaProvider returns a schema provider using the default (contracts) registry.
// This is the fallback used when no registry is explicitly injected (e.g. in tests
// that predate the schema package).
func GetDefaultSchemaProvider() SchemaProvider {
	if contracts.DefaultRegistry() != nil {
		return NewContractV2SchemaProvider(contracts.DefaultRegistry())
	}
	// Last-resort: build from the canonical ecommerce definition directly
	return NewContractV2SchemaProvider(schema.New(schema.EcommerceDefinition()))
}
