package promptcompose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeIsDeterministic(t *testing.T) {
	base, user, omr := loadTestPrompts(t)
	c1 := Compose(base, user, omr)
	c2 := Compose(base, user, omr)
	c3 := Compose(base, user, omr)
	if c1.Content != c2.Content || c2.Content != c3.Content {
		t.Fatal("Compose output is not deterministic across 3 calls")
	}
}

func TestComposeNoAbsolutePaths(t *testing.T) {
	base, user, omr := loadTestPrompts(t)
	composed := Compose(base, user, omr)
	absIndicators := []string{"/Users/", "/home/", "/tmp/", "/var/", "C:\\"}
	for _, indicator := range absIndicators {
		if strings.Contains(composed.Content, indicator) {
			t.Fatalf("composed prompt contains absolute path pattern: %s", indicator)
		}
	}
}

func TestComposeNoDynamicValues(t *testing.T) {
	base, user, omr := loadTestPrompts(t)
	composed := Compose(base, user, omr)
	// Check for timestamp-like patterns
	if strings.Contains(composed.Content, "20") && strings.Contains(composed.Content, "-") && strings.Contains(composed.Content, "T") {
		// Be more precise: match actual ISO timestamp pattern
		content := composed.Content
		if strings.Contains(content, "T") {
			for _, word := range strings.Fields(content) {
				if len(word) >= 19 && strings.Count(word, "-") >= 2 && strings.Contains(word, "T") {
					t.Fatalf("composed prompt contains timestamp-like pattern: %q", word)
				}
			}
		}
	}
	// Check for environment variable references that should be resolved
	envIndicators := []string{"${HOME}", "$HOME", "${USER}", "$USER"}
	for _, indicator := range envIndicators {
		if strings.Contains(composed.Content, indicator) {
			t.Fatalf("composed prompt contains unresolved env var: %s", indicator)
		}
	}
}

func loadTestPrompts(t *testing.T) (string, string, string) {
	basePath := filepath.Join("..", "..", "assets", "prompts", "reasonix-base-464d494.md")
	userPath := filepath.Join("..", "..", "assets", "prompts", "review-task-protocol.zh.md")
	omrPath := filepath.Join("..", "..", "assets", "prompts", "orchestrator.zh.md")

	data, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("required prompt file not found: %s", basePath)
	}
	base := string(data)

	data, err = os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("required prompt file not found: %s", userPath)
	}
	user := string(data)

	data, err = os.ReadFile(omrPath)
	if err != nil {
		t.Fatalf("required prompt file not found: %s", omrPath)
	}
	omr := string(data)

	return base, user, omr
}
