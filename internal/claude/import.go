package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// importFile represents a single file to be imported.
type importFile struct {
	SourceRel  string // relative path within the Claude source directory
	TargetRel  string // relative path within the OMR target directory
	Content    []byte
	SourceDesc string // human-readable description (e.g., ".claude/rules/")
	TargetDesc string // human-readable description (e.g., ".reasonix/rules/")
}

// importFiles handles the generic import flow: discover → conflict check → write → rollback.
// It is used by all sub-commands (rules, skills, agents, mcp, hooks).
func importFiles(opts Options, files []importFile) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	report := Report{Root: root}

	if len(files) == 0 {
		report.NoOp = true
		return report
	}

	var changes []Change
	hasConflict := false

	for _, f := range files {
		targetPath := filepath.Join(root, filepath.FromSlash(f.TargetRel))
		existingHash := fileHash(targetPath)
		fileHash := contentHash(f.Content)

		if existingHash != "" {
			if existingHash == fileHash {
				changes = append(changes, Change{
					Path:   f.TargetRel,
					Action: "SKIP",
					Detail: "content unchanged",
				})
				continue
			}
			if !opts.Force {
				hasConflict = true
				report.Conflicts = append(report.Conflicts,
					fmt.Sprintf("%s already exists with different content (use --force to overwrite)", f.TargetRel))
				continue
			}
		}

		changes = append(changes, Change{
			Path:   f.TargetRel,
			Action: "IMPORT",
			Detail: fmt.Sprintf("from %s", f.SourceDesc+f.SourceRel),
		})
	}

	if hasConflict {
		report.Changes = changes
		return report
	}

	report.Changes = changes

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

	if opts.DryRun {
		return report
	}

	// Execute writes with rollback
	written := map[string][]byte{}
	rollback := func() {
		for path, oldContent := range written {
			if len(oldContent) == 0 {
				os.Remove(path)
			} else {
				os.WriteFile(path, oldContent, 0o644)
			}
		}
	}

	for _, f := range files {
		targetPath := filepath.Join(root, filepath.FromSlash(f.TargetRel))
		oldContent, _ := os.ReadFile(targetPath)
		written[targetPath] = oldContent

		targetDir := filepath.Dir(targetPath)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			rollback()
			report.Errors = append(report.Errors, fmt.Sprintf("create dir for %s: %v", f.TargetRel, err))
			return report
		}
		if err := os.WriteFile(targetPath, f.Content, 0o644); err != nil {
			rollback()
			report.Errors = append(report.Errors, fmt.Sprintf("write %s: %v", f.TargetRel, err))
			return report
		}
	}

	report.Written = true
	return report
}

// ImportRules imports .claude/rules/*.md into .reasonix/rules/.
func ImportRules(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	rules, err := DiscoverRules(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}
	}
	var files []importFile
	for _, r := range rules {
		files = append(files, importFile{
			SourceRel:  r.SourceRel,
			TargetRel:  filepath.Join(OMRRulesDir, r.SourceRel),
			Content:    r.Content,
			SourceDesc: ".claude/rules/",
			TargetDesc: ".reasonix/rules/",
		})
	}
	return importFiles(opts, files)
}

// SkillFile represents a Claude skill file that maps to a Reasonix skill.
type SkillFile struct {
	Name    string // skill identifier
	Content []byte
}

// DiscoverSkills finds all files in .claude/skills/.
func DiscoverSkills(root string) ([]SkillFile, error) {
	skillsDir := filepath.Join(root, filepath.FromSlash(SkillsDir))
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var skills []SkillFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(skillsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read skill %q: %w", entry.Name(), err)
		}
		name := entry.Name()
		if ext := filepath.Ext(name); ext != "" {
			name = strings.TrimSuffix(name, ext)
		}
		skills = append(skills, SkillFile{Name: name, Content: data})
	}
	return skills, nil
}

// ImportSkills imports .claude/skills/* into .reasonix/skills/<name>/SKILL.md.
func ImportSkills(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	skills, err := DiscoverSkills(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}
	}
	var files []importFile
	for _, s := range skills {
		targetRel := filepath.Join(OMRSkillsDir, s.Name, "SKILL.md")
		files = append(files, importFile{
			SourceRel:  s.Name,
			TargetRel:  targetRel,
			Content:    s.Content,
			SourceDesc: ".claude/skills/",
			TargetDesc: ".reasonix/skills/",
		})
	}
	return importFiles(opts, files)
}

// AgentFile represents a Claude agent config file.
type AgentFile struct {
	Name    string // agent identifier
	Content []byte
}

// DiscoverAgents finds all files in .claude/agents/.
func DiscoverAgents(root string) ([]AgentFile, error) {
	agentsDir := filepath.Join(root, filepath.FromSlash(AgentsDir))
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var agents []AgentFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read agent %q: %w", entry.Name(), err)
		}
		name := entry.Name()
		if ext := filepath.Ext(name); ext != "" {
			name = strings.TrimSuffix(name, ext)
		}
		agents = append(agents, AgentFile{Name: name, Content: data})
	}
	return agents, nil
}

// ImportAgents imports .claude/agents/* into .reasonix/skills/<name>/ (as OMR Profile).
func ImportAgents(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	agents, err := DiscoverAgents(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}
	}
	var files []importFile
	for _, a := range agents {
		targetRel := filepath.Join(OMRSkillsDir, "omr-"+a.Name, "SKILL.md")
		files = append(files, importFile{
			SourceRel:  a.Name,
			TargetRel:  targetRel,
			Content:    a.Content,
			SourceDesc: ".claude/agents/",
			TargetDesc: ".reasonix/skills/omr-/",
		})
	}
	return importFiles(opts, files)
}

// ImportMCP imports .claude/mcp.json as-is into .reasonix/.
func ImportMCP(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(MCPFile))
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{Root: root, NoOp: true}
		}
		return Report{Root: root, Errors: []string{err.Error()}}
	}

	targetRel := ".reasonix/mcp.json"
	files := []importFile{{
		SourceRel:  "mcp.json",
		TargetRel:  targetRel,
		Content:    data,
		SourceDesc: ".claude/",
		TargetDesc: ".reasonix/",
	}}
	return importFiles(opts, files)
}

// ImportHooks converts .claude/hooks/* into strategy prompts in .reasonix/rules/.
func ImportHooks(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	hooksDir := filepath.Join(root, filepath.FromSlash(HooksDir))
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{Root: root, NoOp: true}
		}
		return Report{Root: root, Errors: []string{err.Error()}}
	}

	var files []importFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(hooksDir, entry.Name()))
		if err != nil {
			return Report{Root: root, Errors: []string{fmt.Sprintf("read hook %q: %v", entry.Name(), err)}}
		}
		// Convert hook into a strategy prompt rule
		promptContent := append([]byte("# Imported Claude Hook: "+entry.Name()+"\n\n"), data...)
		ruleName := "hook-" + entry.Name()
		if ext := filepath.Ext(ruleName); ext != "" {
			ruleName = ruleName[:len(ruleName)-len(ext)] + ".md"
		} else {
			ruleName += ".md"
		}
		files = append(files, importFile{
			SourceRel:  entry.Name(),
			TargetRel:  filepath.Join(OMRRulesDir, ruleName),
			Content:    promptContent,
			SourceDesc: ".claude/hooks/",
			TargetDesc: ".reasonix/rules/ (converted)",
		})
	}
	return importFiles(opts, files)
}

// ImportAll imports all Claude configuration types at once.
func ImportAll(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	opts.ProjectDir = root

	// Collect all sub-reports
	reports := []Report{
		ImportRules(opts),
		ImportSkills(opts),
		ImportAgents(opts),
		ImportMCP(opts),
		ImportHooks(opts),
	}

	merged := Report{Root: root}
	allNoOp := true
	for _, r := range reports {
		merged.Changes = append(merged.Changes, r.Changes...)
		merged.Warnings = append(merged.Warnings, r.Warnings...)
		merged.Conflicts = append(merged.Conflicts, r.Conflicts...)
		merged.Errors = append(merged.Errors, r.Errors...)
		if r.Written {
			merged.Written = true
		}
		if !r.NoOp {
			allNoOp = false
		}
	}
	merged.NoOp = allNoOp
	return merged
}
