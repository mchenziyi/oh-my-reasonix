package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newClaudeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Create .git to make ProjectRoot work
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeClaudeRule(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(RulesDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportRulesDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	report := ImportRules(Options{ProjectDir: root, DryRun: true})
	if report.Errors != nil {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run should show pending changes")
	}
	if len(report.Changes) != 1 || report.Changes[0].Action != "IMPORT" {
		t.Fatalf("expected 1 IMPORT change, got: %v", report.Changes)
	}

	// Verify no files were written
	targetPath := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")
	if _, err := os.Stat(targetPath); err == nil {
		t.Fatal("dry-run should not write files")
	}
}

func TestImportRulesWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	report := ImportRules(Options{ProjectDir: root})
	if report.Errors != nil {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if !report.Written {
		t.Fatal("expected files to be written")
	}

	targetPath := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target file not written: %v", err)
	}
	if string(data) != "# Style Guide\nUse tabs.\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestImportRulesIdempotent(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	// First import
	report1 := ImportRules(Options{ProjectDir: root})
	if !report1.Written {
		t.Fatal("expected first import to write")
	}

	// Second import - should be no-op
	report2 := ImportRules(Options{ProjectDir: root})
	if !report2.NoOp {
		t.Fatalf("expected second import to be no-op, got changes: %v", report2.Changes)
	}
}

func TestImportRulesConflictDetected(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Claude Style\n")

	// Create existing but different file in target
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "style.md"), []byte("# Different Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := ImportRules(Options{ProjectDir: root})
	if len(report.Conflicts) == 0 {
		t.Fatal("expected conflict")
	}
	if report.Written {
		t.Fatal("should not write on conflict")
	}
	if !strings.Contains(report.Conflicts[0], "already exists") {
		t.Fatalf("expected 'already exists' conflict, got: %v", report.Conflicts)
	}
}

func TestImportRulesForceOverwritesConflict(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Claude Style\n")

	// Existing different content
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "style.md"), []byte("# Different Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := ImportRules(Options{ProjectDir: root, Force: true})
	if len(report.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if !report.Written {
		t.Fatal("expected force to write")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Claude Style\n" {
		t.Fatalf("expected overwritten content, got: %q", data)
	}
}

func TestImportRulesMultipleRules(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style\n")
	writeClaudeRule(t, root, "security.md", "# Security\n")

	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	if len(report.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(report.Changes), report.Changes)
	}
}

func TestImportRulesNoClaudeDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportRules(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op when no .claude/rules exists")
	}
}

func TestImportRulesRollbackOnWriteFailure(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "a.md", "# A\n")
	writeClaudeRule(t, root, "b.md", "# B\n")

	// First write succeeds
	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected first import to succeed")
	}

	// Change rules
	writeClaudeRule(t, root, "a.md", "# A v2\n")
	writeClaudeRule(t, root, "b.md", "# B v2\n")

	// Make target directory read-only so AtomicWrite temp file creation fails
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	os.Chmod(targetDir, 0o555)

	report2 := ImportRules(Options{ProjectDir: root, Force: true})
	if len(report2.Errors) == 0 {
		t.Fatal("expected write errors")
	}

	// Restore permissions for assertions
	os.Chmod(targetDir, 0o755)

	// Verify rollback: files should still be original content
	dataA, _ := os.ReadFile(filepath.Join(targetDir, "a.md"))
	if string(dataA) != "# A\n" {
		t.Fatalf("expected rollback of a.md to %q, got %q", "# A\n", string(dataA))
	}
	dataB, _ := os.ReadFile(filepath.Join(targetDir, "b.md"))
	if string(dataB) != "# B\n" {
		t.Fatalf("expected rollback of b.md to %q, got %q", "# B\n", string(dataB))
	}
}

func TestDiscoverRulesIgnoresNonMdFiles(t *testing.T) {
	root := newClaudeProject(t)
	rulesDir := filepath.Join(root, filepath.FromSlash(RulesDir))
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("text"), 0o644)
	os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("# Style"), 0o644)

	rules, err := DiscoverRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].SourceRel != "style.md" {
		t.Fatalf("expected 1 rule, got %d: %v", len(rules), rules)
	}
}

// ─── Helpers for skills/agents/mcp/hooks tests ───

func writeClaudeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(SkillsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClaudeAgent(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(AgentsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClaudeMCP(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(".claude"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClaudeHook(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(HooksDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ─── ImportSkills tests ───

func TestImportSkillsDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeSkill(t, root, "my-skill.yaml", "name: my-skill\ndescription: A test skill\n")

	report := ImportSkills(Options{ProjectDir: root, DryRun: true})
	if report.Errors != nil {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run should show pending changes")
	}
}

func TestImportSkillsWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeSkill(t, root, "my-skill.yaml", "name: my-skill\n")

	report := ImportSkills(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	targetPath := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "my-skill", "SKILL.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target not written: %v", err)
	}
	if string(data) != "name: my-skill\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestImportSkillsNoClaudeDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportSkills(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op")
	}
}

// ─── ImportAgents tests ───

func TestImportAgentsDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeAgent(t, root, "helper.json", `{"name":"helper","tools":["read"]}`)

	report := ImportAgents(Options{ProjectDir: root, DryRun: true})
	if len(report.Changes) != 1 || report.Changes[0].Action != "IMPORT" {
		t.Fatalf("expected 1 IMPORT, got: %v", report.Changes)
	}
}

func TestImportAgentsWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeAgent(t, root, "helper.json", `{"name":"helper"}`)

	report := ImportAgents(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	targetPath := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "omr-helper", "SKILL.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target not written: %v", err)
	}
	content := string(data)
	for _, marker := range []string{"---", "name: \"omr-helper\"", "runAs: subagent", "read-only: false", "{\"name\":\"helper\"}"} {
		if !strings.Contains(content, marker) {
			t.Fatalf("expected imported Skill frontmatter/body marker %q, got %q", marker, content)
		}
	}
}

func TestImportAgentsNoClaudeDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportAgents(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op")
	}
}

// ─── ImportMCP tests ───

func TestImportMCPDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeMCP(t, root, `{"mcpServers":{"server1":{"command":"node"}}}`)

	report := ImportMCP(Options{ProjectDir: root, DryRun: true})
	if len(report.Changes) != 1 {
		t.Fatalf("expected 1 change, got: %v", report.Changes)
	}
}

func TestImportMCPWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeMCP(t, root, `{"mcpServers":{"test":{"command":"test"}}}`)

	report := ImportMCP(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	targetPath := filepath.Join(root, ".reasonix", "mcp.json")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target not written: %v", err)
	}
	if !strings.Contains(string(data), "mcpServers") {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestImportMCPNoFile(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportMCP(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op when no .claude/mcp.json")
	}
}

// ─── ImportHooks tests ───

func TestImportHooksDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "pre-tool.js", "console.log('pre-tool')")

	report := ImportHooks(Options{ProjectDir: root, DryRun: true})
	if len(report.Changes) != 1 || report.Changes[0].Action != "IMPORT" {
		t.Fatalf("expected 1 IMPORT, got: %v", report.Changes)
	}
}

func TestImportHooksWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "pre-tool.js", "console.log('pre-tool')")

	report := ImportHooks(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	// Hook becomes a .md rule in .reasonix/rules/
	rulesDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 rule file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".md") {
		t.Fatalf("expected .md extension, got %q", entries[0].Name())
	}
}

func TestImportHooksNoDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportHooks(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op")
	}
}

// ─── ImportAll tests ───

func TestImportAllCombinesAllTypes(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style")
	writeClaudeSkill(t, root, "skill.yaml", "name: skill")
	writeClaudeAgent(t, root, "agent.json", `{"name":"agent"}`)
	writeClaudeMCP(t, root, `{"mcpServers":{"s":{"command":"c"}}}`)
	writeClaudeHook(t, root, "hook.js", "hook content")

	report := ImportAll(Options{ProjectDir: root, DryRun: true})
	if report.NoOp {
		t.Fatal("expected changes")
	}
	// Should have 5 changes (rules, skills, agents, mcp, hooks)
	if len(report.Changes) != 5 {
		t.Fatalf("expected 5 changes for all types, got %d: %v", len(report.Changes), report.Changes)
	}
}

func TestImportAllWrite(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style")
	writeClaudeMCP(t, root, `{}`)

	report := ImportAll(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write for ImportAll")
	}
}

func TestImportAllRollsBackWhenLaterSourceIsInvalid(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style")
	writeClaudeMCP(t, root, `{invalid`)

	report := ImportAll(Options{ProjectDir: root})
	if len(report.Errors) == 0 {
		t.Fatal("expected ImportAll to fail on invalid MCP")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")); err == nil {
		t.Fatal("ImportAll must not leave earlier rule writes after a later validation failure")
	}
}

// ─── FIX-05+06: Claude import security and compatibility tests ───

func TestImportMCPRejectsInvalidJSON(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeMCP(t, root, `{"invalid json`)

	report := ImportMCP(Options{ProjectDir: root})
	if len(report.Errors) == 0 {
		t.Fatal("expected error for invalid MCP JSON")
	}
	if !strings.Contains(report.Errors[0], "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' error, got: %v", report.Errors)
	}
}

func TestImportRulesAtomicWrite(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style")

	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}

	targetFile := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Style" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestImportRulesFilePermissionsRestoredOnRollback(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "a.md", "# A\n")
	writeClaudeRule(t, root, "b.md", "# B\n")

	// First import succeeds
	report1 := ImportRules(Options{ProjectDir: root})
	if !report1.Written {
		t.Fatal("expected first import to succeed")
	}

	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	targetFile := filepath.Join(targetDir, "a.md")
	// Store original permissions
	origInfo, _ := os.Stat(targetFile)
	origMode := origInfo.Mode().Perm()

	// Change source files
	writeClaudeRule(t, root, "a.md", "# A v2\n")
	writeClaudeRule(t, root, "b.md", "# B v2\n")

	// Make the directory read-only so AtomicWrite fails (can't create temp file)
	os.Chmod(targetDir, 0o555)

	// Import with force should fail
	report2 := ImportRules(Options{ProjectDir: root, Force: true})
	if len(report2.Errors) == 0 {
		t.Fatal("expected write error")
	}

	// Restore permissions so we can read
	os.Chmod(targetDir, 0o755)

	// After rollback, file content should be original
	data, _ := os.ReadFile(targetFile)
	if string(data) != "# A\n" {
		t.Fatalf("content not rolled back: got %q", string(data))
	}

	// File permissions should be unchanged
	afterInfo, _ := os.Stat(targetFile)
	if afterInfo.Mode().Perm() != origMode {
		t.Fatalf("file permission changed after rollback: was %o, got %o", origMode, afterInfo.Mode().Perm())
	}
}

func TestImportRulesZeroByteFile(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "empty.md", "")

	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write for zero-byte rule")
	}

	targetFile := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "empty.md")
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected zero-byte file, got %d bytes", len(data))
	}
}

func TestImportMCPEmptyFile(t *testing.T) {
	root := newClaudeProject(t)
	// Empty file is invalid JSON
	writeClaudeMCP(t, root, "")

	report := ImportMCP(Options{ProjectDir: root})
	if len(report.Errors) == 0 {
		t.Fatal("expected error for empty MCP file")
	}
}

func TestImportHooksPrefixDisclaimer(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "pre-tool.js", "console.log('test')")

	report := ImportHooks(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}

	// Verify disclaimer is present
	rulesDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	entries, _ := os.ReadDir(rulesDir)
	if len(entries) == 0 {
		t.Fatal("no rule files created")
	}
	data, _ := os.ReadFile(filepath.Join(rulesDir, entries[0].Name()))
	content := string(data)
	if !strings.Contains(content, "策略提示转换") {
		t.Fatalf("expected conversion disclaimer in hook output, got: %s", content)
	}
	if !strings.Contains(content, "不保证等价于运行时 Hook") {
		t.Fatalf("expected runtime disclaimer in hook output, got: %s", content)
	}
}
