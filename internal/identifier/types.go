package identifier

type IdentifierType string

const (
	TypeOrderNumber    IdentifierType = "order_number"
	TypeTrackingID     IdentifierType = "tracking_id"
	TypePaymentRef     IdentifierType = "payment_reference"
	TypeCustomerNumber IdentifierType = "customer_number"
	TypeEmail          IdentifierType = "email"
	TypePhone          IdentifierType = "phone"
	TypeUnknownToken   IdentifierType = "unknown_token"
)

type DetectedIdentifier struct {
	Type            IdentifierType `json:"type"`
	NormalizedValue string         `json:"normalizedValue"`
	Confidence      float64        `json:"confidence"`
}

type GroupSpec struct {
	ResourceType string  `json:"resourceType"`
	MatchField   string  `json:"matchField"`
	Operator     string  `json:"operator"`
	Value        string  `json:"value"`
	Confidence   float64 `json:"confidence"`
}

type ResolutionPlan struct {
	ShouldUseFastPath        bool                 `json:"shouldUseFastPath"`
	Detected                 []DetectedIdentifier `json:"detected"`
	Groups                   []GroupSpec          `json:"groups"`
	QueryShape               QueryShape           `json:"queryShape"`
	NormalizationApplied     []string             `json:"normalizationApplied,omitempty"`
	IdentifierPatternMatched string               `json:"identifierPatternMatched,omitempty"`
	NormalizedInput          string               `json:"normalizedInput,omitempty"`
}
