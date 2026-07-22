package claude

import (
	"fmt"
	"os"
	"path/filepath"
)

// ImportRules imports .claude/rules/*.md into .reasonix/rules/.
// Supports dry-run, conflict detection, and rollback.
func ImportRules(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	report := Report{Root: root}

	// Discover Claude rules
	rules, err := DiscoverRules(root)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	if len(rules) == 0 {
		report.NoOp = true
		return report
	}

	// Target directory
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))

	// Check for conflicts and build changes
	var changes []Change
	hasConflict := false
	for _, rule := range rules {
		targetPath := filepath.Join(targetDir, rule.SourceRel)
		existingHash := fileHash(targetPath)
		ruleHash := contentHash(rule.Content)

		if existingHash != "" {
			if existingHash == ruleHash {
				// Already up-to-date
				changes = append(changes, Change{
					Path:   filepath.Join(OMRRulesDir, rule.SourceRel),
					Action: "SKIP",
					Detail: "content unchanged",
				})
				continue
			}
			if !opts.Force {
				hasConflict = true
				report.Conflicts = append(report.Conflicts,
					fmt.Sprintf("%s already exists with different content (use --force to overwrite)",
						filepath.Join(OMRRulesDir, rule.SourceRel)))
				continue
			}
		}

		changes = append(changes, Change{
			Path:   filepath.Join(OMRRulesDir, rule.SourceRel),
			Action: "IMPORT",
			Detail: fmt.Sprintf("from .claude/rules/%s", rule.SourceRel),
		})
	}

	if hasConflict {
		report.Changes = changes
		return report
	}

	report.Changes = changes

	// Check if anything actually needs writing
	needsWrite := false
	for _, c := range changes {
		if c.Action == "IMPORT" {
			needsWrite = true
			break
		}
	}
	if !needsWrite {
		report.NoOp = true
		return report
	}

	// Dry-run: return plan
	if opts.DryRun {
		return report
	}

	// Execute writes with rollback
	written := map[string][]byte{} // path → old content (empty if new)
	rollback := func() {
		for path, oldContent := range written {
			if len(oldContent) == 0 {
				os.Remove(path)
			} else {
				os.WriteFile(path, oldContent, 0o644)
			}
		}
	}

	for _, rule := range rules {
		targetPath := filepath.Join(targetDir, rule.SourceRel)
		// Save old content for rollback
		oldContent, _ := os.ReadFile(targetPath)
		written[targetPath] = oldContent

		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			rollback()
			report.Errors = append(report.Errors, fmt.Sprintf("create rules dir: %v", err))
			return report
		}
		if err := os.WriteFile(targetPath, rule.Content, 0o644); err != nil {
			rollback()
			report.Errors = append(report.Errors, fmt.Sprintf("write %s: %v", targetPath, err))
			return report
		}
	}

	report.Written = true
	return report
}
