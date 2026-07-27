// Package grillwithdocs provides a pure-Go model for documenting confirmed
// terms, facts, and high-impact decisions into CONTEXT.md and ADR files.
// It enforces dry-run-before-write, idempotency, conflict detection, and
// strict security boundaries (no abs paths, no path traversal, no symlink
// escape, no .reasonix writes, no writes outside project root).
package grillwithdocs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

// slugPattern validates ADR slugs: lowercase alphanumeric with hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][-a-z0-9]*$`)

// TermFact represents a confirmed term or fact to record in CONTEXT.md.
type TermFact struct {
	Term   string
	Fact   string
	Source string
}

// ADRDraft represents an Architecture Decision Record to create.
type ADRDraft struct {
	Slug              string
	Title             string
	Status            string // proposed | accepted | deprecated | superseded
	Context           string
	Decision          string
	Consequences      string
	Alternatives      []string
	ConfirmationBasis string
}

// DocPlan is the output of Plan() — a dry-run preview or the plan to Apply().
type DocPlan struct {
	DryRun       bool
	TermFacts    []TermFact
	ADRs         []ADRDraft
	ExistingADRs int
	Conflicts    []string
}

// Plan performs a dry-run scan: reads existing CONTEXT.md and docs/adr/,
// detects conflicting facts, counts existing ADRs, and returns a DocPlan.
// It does NOT write any files.
func Plan(projectRoot string, facts []TermFact, adrs []ADRDraft) (DocPlan, error) {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		root, err = filepath.Abs(projectRoot)
		if err != nil {
			return DocPlan{}, fmt.Errorf("resolve project root: %w", err)
		}
	}

	plan := DocPlan{
		DryRun:    true,
		TermFacts: facts,
		ADRs:      adrs,
	}

	// Count existing ADRs
	adrDir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(adrDir)
	if err == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				count++
			}
		}
		plan.ExistingADRs = count
	}

	// Detect conflicts in CONTEXT.md
	ctxPath := filepath.Join(root, "CONTEXT.md")
	if data, err := os.ReadFile(ctxPath); err == nil {
		existing := string(data)
		for _, f := range facts {
			if strings.Contains(existing, f.Term) && !strings.Contains(existing, f.Fact) {
				plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("term %q already exists with different fact (proposed: %q)", f.Term, f.Fact))
			}
		}
	}

	return plan, nil
}

// Apply writes the DocPlan to disk. It enforces all security boundaries:
// no absolute paths, no path traversal, no symlink escape, no .reasonix/,
// no writes outside project root. Only writes when DryRun is false.
// It uses a two-phase approach: pre-check all paths/slugs before any write,
// so a validation failure leaves the workspace unmodified.
func Apply(projectRoot string, plan DocPlan) error {
	if plan.DryRun {
		return nil
	}

	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		root, err = filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("resolve project root: %w", err)
		}
	}

	// ---- Phase 1: Pre-check everything before any write ----

	// Pre-check CONTEXT.md path
	if len(plan.TermFacts) > 0 {
		ctxPath := filepath.Join(root, "CONTEXT.md")
		if err := validatePath(root, ctxPath); err != nil {
			return fmt.Errorf("pre-check CONTEXT.md: %w", err)
		}
	}

	// Pre-check all ADR slugs and target paths
	for i, adr := range plan.ADRs {
		if !slugPattern.MatchString(adr.Slug) {
			return fmt.Errorf("invalid ADR slug %q: must match %s", adr.Slug, slugPattern.String())
		}
		number := plan.ExistingADRs + i + 1
		slug := fmt.Sprintf("%04d-%s.md", number, adr.Slug)
		target := filepath.Join(root, "docs", "adr", slug)

		// Verify target is within docs/adr/
		adrDir := filepath.Join(root, "docs", "adr")
		adrRel, err := filepath.Rel(adrDir, target)
		if err != nil || strings.HasPrefix(adrRel, "..") {
			return fmt.Errorf("ADR target %q is outside docs/adr/ directory", target)
		}

		if err := validatePath(root, target); err != nil {
			return fmt.Errorf("security check failed for ADR %q: %w", slug, err)
		}
	}

	// ---- Phase 2: All checks passed, perform writes ----

	// Update CONTEXT.md
	if len(plan.TermFacts) > 0 {
		if err := updateContextMD(root, plan.TermFacts); err != nil {
			return fmt.Errorf("update CONTEXT.md: %w", err)
		}
	}

	// Create ADRs
	for i, adr := range plan.ADRs {
		number := plan.ExistingADRs + i + 1
		slug := fmt.Sprintf("%04d-%s.md", number, adr.Slug)
		target := filepath.Join(root, "docs", "adr", slug)

		content := renderADR(number, adr)
		if err := fileutil.AtomicWrite(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write ADR %q: %w", slug, err)
		}
	}

	return nil
}

// updateContextMD appends new term/fact entries to CONTEXT.md idempotently.
func updateContextMD(root string, facts []TermFact) error {
	ctxPath := filepath.Join(root, "CONTEXT.md")

	if err := validatePath(root, ctxPath); err != nil {
		return err
	}

	var existing string
	if data, err := os.ReadFile(ctxPath); err == nil {
		existing = string(data)
	}

	var newLines []string
	for _, f := range facts {
		entry := fmt.Sprintf("- %s: %s (source: %s)", f.Term, f.Fact, f.Source)
		if !strings.Contains(existing, entry) {
			newLines = append(newLines, entry)
		}
	}

	if len(newLines) == 0 {
		return nil // nothing new to add
	}

	// Append to the existing content (before the file ends)
	content := existing
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "<!-- grill-with-docs -->\n"
	for _, line := range newLines {
		content += line + "\n"
	}

	return fileutil.AtomicWrite(ctxPath, []byte(content), 0o644)
}

// renderADR renders an ADR draft as a Markdown file.
func renderADR(number int, adr ADRDraft) string {
	altText := ""
	for _, a := range adr.Alternatives {
		altText += fmt.Sprintf("- %s\n", a)
	}

	return fmt.Sprintf(`# %s

- **Status**: %s
- **Number**: %04d
- **Confirmation Basis**: %s

## Context

%s

## Decision

%s

## Consequences

%s

## Alternatives

%s
`, adr.Title, adr.Status, number, adr.ConfirmationBasis, adr.Context, adr.Decision, adr.Consequences, altText)
}

// validatePath enforces all security boundaries.
func validatePath(projectRoot, targetPath string) error {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		root, err = filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("resolve project root: %w", err)
		}
	}

	target, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	// Check target is within project root using Rel, not HasPrefix (avoids /project-evil bypass)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("target %q is outside project root %q", targetPath, root)
	}

	// Check for .reasonix
	if strings.HasPrefix(rel, ".reasonix") || strings.HasPrefix(rel, ".reasonix/") {
		return fmt.Errorf("writing to .reasonix is not allowed: %q", rel)
	}

	// Check for symlink escape — inspect every path component
	remaining := target
	for remaining != root && len(remaining) > len(root) {
		if fi, statErr := os.Lstat(remaining); statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(remaining)
			if err != nil {
				return fmt.Errorf("symlink resolution for %q: %w", remaining, err)
			}
			if linkRel, linkErr := filepath.Rel(root, resolved); linkErr != nil || strings.HasPrefix(linkRel, "..") {
				return fmt.Errorf("symlink %q escapes project root to %q", remaining, resolved)
			}
			// Symlink resolved within root, continue checking parent
		}
		remaining = filepath.Dir(remaining)
		if remaining == "." {
			break
		}
	}

	return nil
}
