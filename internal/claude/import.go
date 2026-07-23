package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
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
	type fileState struct {
		content     []byte
		mode        os.FileMode
		contentRead bool // false = file didn't exist
	}
	written := map[string]fileState{}
	rollback := func() {
		for path, state := range written {
			if !state.contentRead {
				os.Remove(path)
			} else {
				os.WriteFile(path, state.content, state.mode)
			}
		}
	}

	for _, f := range files {
		targetPath := filepath.Join(root, filepath.FromSlash(f.TargetRel))
		state := fileState{}
		oldContent, err := os.ReadFile(targetPath)
		if err == nil {
			state.content = oldContent
			state.contentRead = true
			if info, statErr := os.Stat(targetPath); statErr == nil {
				state.mode = info.Mode().Perm()
			} else {
				state.mode = 0o644
			}
		}
		written[targetPath] = state

		targetDir := filepath.Dir(targetPath)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			rollback()
			report.Errors = append(report.Errors, fmt.Sprintf("create dir for %s: %v", f.TargetRel, err))
			return report
		}
		if err := fileutil.AtomicWrite(targetPath, f.Content, 0o644); err != nil {
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
		if err := ValidateSkillFrontmatter(s.Content); err != nil {
			return Report{Root: root, Errors: []string{fmt.Sprintf("skill %q: %v", s.Name, err)}}
		}
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

// ValidateSkillFrontmatter checks that content has valid Reasonix Skill frontmatter.
// Returns an error listing all missing/required fields.
func ValidateSkillFrontmatter(content []byte) error {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return fmt.Errorf("missing frontmatter delimiter ---")
	}
	endIdx := strings.Index(text[len("---\n"):], "\n---")
	if endIdx < 0 {
		return fmt.Errorf("frontmatter delimiter --- not closed")
	}
	frontMatter := text[len("---\n") : len("---\n")+endIdx]
	fields := map[string]string{}
	for _, line := range strings.Split(frontMatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		fields[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	var errs []string
	if fields["name"] == "" {
		errs = append(errs, "frontmatter missing required field: name")
	}
	if fields["description"] == "" {
		errs = append(errs, "frontmatter missing required field: description")
	}
	if ro, ok := fields["read-only"]; ok && ro != "true" && ro != "false" {
		errs = append(errs, fmt.Sprintf("frontmatter read-only must be boolean, got: %q", ro))
	}
	if ra, ok := fields["runAs"]; ok && ra != "subagent" && ra != "manual" {
		errs = append(errs, fmt.Sprintf("frontmatter runAs must be 'subagent' or 'manual', got: %q", ra))
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
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
			Content:    importedAgentSkill(a.Name, a.Content),
			SourceDesc: ".claude/agents/",
			TargetDesc: ".reasonix/skills/omr-/",
		})
	}
	return importFiles(opts, files)
}

// importedAgentSkill wraps Claude agent instructions in the frontmatter required
// by a Reasonix project Skill. The original agent body is preserved verbatim.
func importedAgentSkill(name string, content []byte) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: %q\ndescription: Imported Claude agent profile\ninvocation: manual\nrunAs: subagent\nread-only: false\n---\n\n", "omr-"+name)
	b.Write(content)
	return []byte(b.String())
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

	// Validate JSON before import
	if !json.Valid(data) {
		return Report{Root: root, Errors: []string{
			fmt.Sprintf("%s: invalid JSON — import rejected", MCPFile),
		}}
	}

	// Compatibility analysis
	var warnings []string
	var mcpData map[string]json.RawMessage
	if err := json.Unmarshal(data, &mcpData); err == nil {
		if serversRaw, ok := mcpData["mcpServers"]; ok {
			var servers map[string]map[string]interface{}
			if err := json.Unmarshal(serversRaw, &servers); err == nil {
				for name, srv := range servers {
					compat := fmt.Sprintf("MCP: %s — 原样保留", name)
					// Check command
					if cmd, hasCmd := srv["command"]; hasCmd {
						if cmdStr, ok := cmd.(string); ok {
							if _, lookErr := exec.LookPath(cmdStr); lookErr != nil && !opts.DryRun {
								compat += fmt.Sprintf(", 命令 %q 可能需要额外安装", cmdStr)
							}
						}
					}
					// Redact env values
					if env, hasEnv := srv["env"]; hasEnv {
						if envMap, ok := env.(map[string]interface{}); ok {
							for k := range envMap {
								compat += fmt.Sprintf(", env.%s=***", k)
							}
						}
					}
					warnings = append(warnings, compat)
				}
			}
		}
	}

	targetRel := ".reasonix/mcp.json"
	files := []importFile{{
		SourceRel:  "mcp.json",
		TargetRel:  targetRel,
		Content:    data,
		SourceDesc: ".claude/",
		TargetDesc: ".reasonix/",
	}}
	report := importFiles(opts, files)
	report.Warnings = append(report.Warnings, warnings...)
	return report
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
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(hooksDir, entry.Name()))
		if err != nil {
			return Report{Root: root, Errors: []string{fmt.Sprintf("read hook %q: %v", entry.Name(), err)}}
		}
		text := string(data)

		// Semantic analysis: detect dangerous patterns
		var risks []string
		dangerPatterns := []struct {
			pattern string
			desc    string
		}{
			{"rm -rf", "recursive delete"},
			{"curl |", "pipe from curl"},
			{"sudo ", "escalated privilege"},
			{"/usr/bin/", "absolute binary path"},
			{"/bin/", "absolute binary path"},
			{"chmod 777", "world-writable permission"},
		}
		for _, dp := range dangerPatterns {
			if strings.Contains(text, dp.pattern) {
				risks = append(risks, dp.desc)
			}
		}

		// Sensitive content detection
		secretPatterns := []string{"API_KEY=", "api_key=", "token=", "password=", "secret=", "SECRET="}
		for _, sp := range secretPatterns {
			if strings.Contains(text, sp) {
				risks = append(risks, "contains "+sp+"*** (value redacted)")
			}
		}

		riskNote := "无"
		if len(risks) > 0 {
			riskNote = strings.Join(risks, ", ")
		}

		// Enhanced disclaimer
		disclaimer := fmt.Sprintf("# [策略提示转换] 此文件由 Claude Hook %q 转换而来\n# 不保证等价于运行时 Hook 执行。Hook 的命令执行、阻断、顺序保证等运行时语义已丢失。\n# 原始来源: .claude/hooks/%s\n# 需要人工复核以下风险: %s\n\n",
			entry.Name(), entry.Name(), riskNote)
		promptContent := append([]byte(disclaimer), data...)
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
		warnings = append(warnings, fmt.Sprintf("Hook %q — 已转为策略提示 (已保留语义: 触发条件; 无法保留: 命令执行、阻断、环境修改; 风险: %s)", entry.Name(), riskNote))
	}
	report := importFiles(opts, files)
	report.Warnings = append(report.Warnings, warnings...)
	return report
}

// CommandFile represents a single command file from .claude/commands/.
type CommandFile struct {
	Name      string // command name (without extension)
	SourceRel string // original filename
	Content   []byte
}

// DiscoverCommands finds all text files in .claude/commands/.
func DiscoverCommands(root string) ([]CommandFile, error) {
	cmdDir := filepath.Join(root, filepath.FromSlash(CommandsDir))
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var commands []CommandFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		// Only accept known text extensions
		if ext != "" && ext != ".md" && ext != ".txt" && ext != ".sh" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cmdDir, name))
		if err != nil {
			return nil, fmt.Errorf("read command %q: %w", name, err)
		}
		if len(data) == 0 {
			continue
		}
		// Strip extension for skill name
		cmdName := name
		if ext != "" {
			cmdName = strings.TrimSuffix(name, ext)
		}
		commands = append(commands, CommandFile{
			Name:      cmdName,
			SourceRel: name,
			Content:   data,
		})
	}
	return commands, nil
}

// commandToSkillContent wraps a Claude command in Reasonix skill frontmatter.
func commandToSkillContent(name string, content []byte) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %q\n", "cmd-"+name)
	fmt.Fprintf(&b, "description: Imported Claude command %q\n", name)
	fmt.Fprintf(&b, "runAs: subagent\ninvocation: command\nread-only: false\n---\n\n")
	b.WriteString("# [导入] 此命令从 .claude/commands/ 转换而来，不保证等价于原始执行\n\n")
	b.Write(content)
	return []byte(b.String())
}

// ImportCommands imports .claude/commands/* into .reasonix/skills/cmd-<name>/SKILL.md.
func ImportCommands(opts Options) Report {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{Root: opts.ProjectDir, Errors: []string{err.Error()}}
	}
	commands, err := DiscoverCommands(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}
	}
	// Check for duplicate names
	seen := map[string]bool{}
	var files []importFile
	for _, c := range commands {
		if seen[c.Name] {
			return Report{Root: root, Conflicts: []string{
				fmt.Sprintf("duplicate command name %q in .claude/commands/", c.Name),
			}}
		}
		seen[c.Name] = true
		targetRel := filepath.Join(OMRSkillsDir, "cmd-"+c.Name, "SKILL.md")
		files = append(files, importFile{
			SourceRel:  c.SourceRel,
			TargetRel:  targetRel,
			Content:    commandToSkillContent(c.Name, c.Content),
			SourceDesc: ".claude/commands/",
			TargetDesc: ".reasonix/skills/",
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

	// Preflight every source before writing any category. This prevents a later
	// invalid MCP/Hook source from leaving earlier categories partially imported.
	planOpts := opts
	planOpts.DryRun = true
	planned := []Report{
		ImportRules(planOpts),
		ImportSkills(planOpts),
		ImportAgents(planOpts),
		ImportCommands(planOpts),
		ImportMCP(planOpts),
		ImportHooks(planOpts),
	}
	plannedReport := mergeReports(root, planned)
	if len(plannedReport.Errors) > 0 || len(plannedReport.Conflicts) > 0 || opts.DryRun {
		return plannedReport
	}

	snapshots := snapshotImportFiles(root, plannedReport.Changes)
	reports := []Report{
		ImportRules(opts),
		ImportSkills(opts),
		ImportAgents(opts),
		ImportCommands(opts),
		ImportMCP(opts),
		ImportHooks(opts),
	}

	merged := mergeReports(root, reports)
	if len(merged.Errors) > 0 || len(merged.Conflicts) > 0 {
		restoreImportFiles(snapshots)
	}
	return merged
}

type importSnapshot struct {
	content []byte
	mode    os.FileMode
	exists  bool
}

func mergeReports(root string, reports []Report) Report {
	merged := Report{Root: root, NoOp: true}
	for _, r := range reports {
		merged.Changes = append(merged.Changes, r.Changes...)
		merged.Warnings = append(merged.Warnings, r.Warnings...)
		merged.Conflicts = append(merged.Conflicts, r.Conflicts...)
		merged.Errors = append(merged.Errors, r.Errors...)
		merged.Written = merged.Written || r.Written
		if !r.NoOp {
			merged.NoOp = false
		}
	}
	return merged
}

func snapshotImportFiles(root string, changes []Change) map[string]importSnapshot {
	snapshots := make(map[string]importSnapshot)
	for _, change := range changes {
		if change.Action != "IMPORT" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(change.Path))
		state := importSnapshot{}
		if data, err := os.ReadFile(path); err == nil {
			state.content = data
			state.exists = true
			if info, statErr := os.Stat(path); statErr == nil {
				state.mode = info.Mode().Perm()
			}
		}
		snapshots[path] = state
	}
	return snapshots
}

func restoreImportFiles(snapshots map[string]importSnapshot) {
	for path, state := range snapshots {
		if !state.exists {
			_ = os.Remove(path)
			continue
		}
		mode := state.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := fileutil.AtomicWrite(path, state.content, mode); err == nil {
			_ = os.Chmod(path, mode)
		}
	}
}
