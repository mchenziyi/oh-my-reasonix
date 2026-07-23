package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSkillFrontmatterValid(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nread-only: false\nrunAs: subagent\n---\n\ncontent here\n")
	warns, err := ValidateSkillFrontmatter(content)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	if len(warns) > 0 {
		t.Fatalf("expected no warnings, got: %v", warns)
	}
}

func TestValidateSkillFrontmatterMissingName(t *testing.T) {
	content := []byte("---\ndescription: A test skill\n---\n\ncontent\n")
	_, err := ValidateSkillFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateSkillFrontmatterInvalidReadOnly(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nread-only: yes\n---\n\ncontent\n")
	_, err := ValidateSkillFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for invalid read-only")
	}
}

func TestValidateSkillFrontmatterInvalidRunAs(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nrunAs: invalid\n---\n\ncontent\n")
	_, err := ValidateSkillFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for invalid runAs")
	}
}

func TestValidateSkillFrontmatterMissingDelimiter(t *testing.T) {
	content := []byte("no frontmatter here\n")
	_, err := ValidateSkillFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for missing delimiter")
	}
}

func TestValidateSkillFrontmatterUnknownField(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nfoo: bar\n---\n\ncontent\n")
	warns, err := ValidateSkillFrontmatter(content)
	if err != nil {
		t.Fatalf("expected valid despite unknown field, got: %v", err)
	}
	found := false
	for _, w := range warns {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected warning for unknown field")
	}
}

func TestImportSkillRejectsInvalidFrontmatter(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeSkill(t, root, "bad-skill", "no frontmatter here\n")
	report := ImportSkills(Options{ProjectDir: root})
	if len(report.Errors) == 0 {
		t.Fatal("expected error for invalid frontmatter")
	}
	// Verify no file was written
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "bad-skill", "SKILL.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("expected no file written for invalid frontmatter")
	}
}

func TestImportAgentWarnsOnRawFrontmatter(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeAgent(t, root, "test-agent", "---\nname: test\ndescription: an agent\n---\n\nagent instructions\n")
	report := ImportAgents(Options{ProjectDir: root})
	// Should still import even with valid frontmatter
	if len(report.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "omr-test-agent", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected agent to be imported: %v", err)
	}
}

func TestImportAgentRejectsInvalidFrontmatter(t *testing.T) {
	root := newClaudeProject(t)
	// Agent content without --- delimiters is accepted as-is (no validation)
	writeClaudeAgent(t, root, "plain-agent", "plain instructions\n")
	report := ImportAgents(Options{ProjectDir: root})
	if len(report.Errors) > 0 {
		t.Fatalf("unexpected errors for plain agent: %v", report.Errors)
	}
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "omr-plain-agent", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected plain agent to be imported: %v", err)
	}
}

func TestImportAgentRejectsInvalidFrontmatterDelimiters(t *testing.T) {
	root := newClaudeProject(t)
	// Agent content WITH --- but invalid frontmatter should be rejected
	writeClaudeAgent(t, root, "bad-agent", "---\ninvalid\n---\n\ninstructions\n")
	report := ImportAgents(Options{ProjectDir: root})
	found := false
	for _, e := range report.Errors {
		if len(e) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected error for agent with invalid frontmatter")
	}
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "omr-bad-agent", "SKILL.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("expected no file written for agent with invalid frontmatter")
	}
}
