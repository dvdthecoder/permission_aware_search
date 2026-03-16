package semantic

// IntentFramer abstracts intent framing so teams can plug custom intent
// classifiers/extractors without changing analyzer orchestration.
type IntentFramer interface {
	Name() string
	Normalize(message string) string
	Frame(message, contractVersion, resourceHint string) ParseResult
}

type DeterministicIntentFramer struct{}

func NewDeterministicIntentFramer() *DeterministicIntentFramer {
	return &DeterministicIntentFramer{}
}

func (f *DeterministicIntentFramer) Name() string { return "deterministic-intent-framer" }

func (f *DeterministicIntentFramer) Normalize(message string) string {
	return normalizePrompt(message)
}

func (f *DeterministicIntentFramer) Frame(message, contractVersion, resourceHint string) ParseResult {
	return ParseNaturalLanguage(message, contractVersion, resourceHint)
}
