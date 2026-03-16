package semantic

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type goldenPrompt struct {
	Category string
	Text     string
}

func TestGoldenPromptSetCoverage(t *testing.T) {
	prompts := loadGoldenPrompts(t)
	if len(prompts) < 40 {
		t.Fatalf("expected large golden prompt set, got %d", len(prompts))
	}

	a := NewSLMLocalAnalyzer()
	for _, gp := range prompts {
		normalized := normalizeGoldenPrompt(gp.Text)
		res, err := a.Analyze(context.Background(), AnalyzeRequest{Message: normalized, ContractVersion: "v2"})
		if err != nil {
			t.Fatalf("analyze failed for prompt %q: %v", normalized, err)
		}
		if res.IntentCategory == "" {
			t.Fatalf("missing intentCategory for prompt %q", normalized)
		}
		if res.ResourceType == "" {
			t.Fatalf("missing resourceType for prompt %q", normalized)
		}
		if requiresDeterministicResolution(normalized) && res.ClarificationNeeded {
			t.Fatalf("expected resolved prompt but got ambiguity: %q", normalized)
		}
	}
}

func loadGoldenPrompts(t *testing.T) []goldenPrompt {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	path := filepath.Join(root, "testdata", "internal_support_semantic_layer_golden_prompts.md")

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open golden prompt file failed: %v", err)
	}
	defer f.Close()

	out := []goldenPrompt{}
	category := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "## ") {
			category = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		if strings.HasPrefix(line, "-") {
			text := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if text != "" {
				out = append(out, goldenPrompt{Category: category, Text: text})
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan golden prompt file failed: %v", err)
	}
	return out
}

func normalizeGoldenPrompt(in string) string {
	replacer := strings.NewReplacer(
		"7823419", "ORD-000001",
		"CUST-8821", "CUST-000001",
		"john.smith@gmail.com", "customer00001@tenant-a.example.com",
		"+1 415 555 9012", "+1 415 555 0001",
		"RMA-22091", "RMA-000001",
	)
	return replacer.Replace(in)
}

func requiresDeterministicResolution(prompt string) bool {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "ord-") || strings.Contains(lower, "cust-") || strings.Contains(lower, "@") || strings.Contains(lower, "tracking") {
		return true
	}
	if strings.Contains(lower, "return") || strings.Contains(lower, "refund") {
		return true
	}
	return false
}
