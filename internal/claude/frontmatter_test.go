package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSkillFrontmatterValid(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nread-only: false\nrunAs: subagent\n---\n\ncontent here\n")
	if err := ValidateSkillFrontmatter(content); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateSkillFrontmatterMissingName(t *testing.T) {
	content := []byte("---\ndescription: A test skill\n---\n\ncontent\n")
	if err := ValidateSkillFrontmatter(content); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateSkillFrontmatterInvalidReadOnly(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nread-only: yes\n---\n\ncontent\n")
	if err := ValidateSkillFrontmatter(content); err == nil {
		t.Fatal("expected error for invalid read-only")
	}
}

func TestValidateSkillFrontmatterInvalidRunAs(t *testing.T) {
	content := []byte("---\nname: \"test\"\ndescription: A test skill\nrunAs: invalid\n---\n\ncontent\n")
	if err := ValidateSkillFrontmatter(content); err == nil {
		t.Fatal("expected error for invalid runAs")
	}
}

func TestValidateSkillFrontmatterMissingDelimiter(t *testing.T) {
	content := []byte("no frontmatter here\n")
	if err := ValidateSkillFrontmatter(content); err == nil {
		t.Fatal("expected error for missing delimiter")
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
	// Agent file with raw frontmatter-like content
	writeClaudeAgent(t, root, "test-agent", "---\nname: test\n---\n\nagent instructions\n")
	report := ImportAgents(Options{ProjectDir: root})
	if len(report.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	// Agent should still be imported (frontmatter check is advisory)
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "omr-test-agent", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected agent to be imported: %v", err)
	}
}
