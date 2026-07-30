// Package commentchecker provides an offline comment quality checker.
// It implements deterministic rules that operate on file snapshots,
// producing stable, JSON-serializable reports.
//
// The checker is deliberately read-only: it never modifies source files.
// Sensitive content (keys, tokens, passwords) is redacted before it appears
// in the report.
package commentchecker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SchemaVersion is the current report schema version.
const SchemaVersion = 1

// Severity levels for findings.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityBlocking Severity = "blocking"
)

// RuleID identifies a comment-checker rule.
type RuleID string

const (
	R001 RuleID = "R001" // Debug/placeholder markers (TODO/FIXME/XXX)
	R002 RuleID = "R002" // Empty or placeholder comments
	R003 RuleID = "R003" // Comment-code over-similarity (warning only)
	R004 RuleID = "R004" // Credential/key leak (blocking, redacted)
	R005 RuleID = "R005" // Path safety
)

// RuleInfo maps RuleID to a human-readable rule description.
var RuleInfo = map[RuleID]string{
	R001: "Detect debug or placeholder markers (TODO, FIXME, XXX) not in the allowlist",
	R002: "Detect empty or placeholder-only comments",
	R003: "Detect comments that are overly similar to adjacent code",
	R004: "Detect suspected credentials, tokens, or passwords in comments",
	R005: "Reject files outside the allowed project root",
}

// Finding represents a single comment quality issue.
type Finding struct {
	File           string   `json:"file"`
	Line           int      `json:"line"`
	Column         int      `json:"column,omitempty"`
	RuleID         RuleID   `json:"rule_id"`
	Severity       Severity `json:"severity"`
	Message        string   `json:"message"`
	RedactedDetail string   `json:"redacted_detail,omitempty"`
	Suggestion     string   `json:"suggestion,omitempty"`
}

// Summary aggregates finding counts.
type Summary struct {
	TotalFiles    int `json:"total_files"`
	TotalFindings int `json:"total_findings"`
	BlockingCount int `json:"blocking_count"`
	WarningCount  int `json:"warning_count"`
	InfoCount     int `json:"info_count"`
	SkippedFiles  int `json:"skipped_files"`
}

// Report is the top-level result of a comment-check run.
type Report struct {
	SchemaVersion int       `json:"schema_version"`
	ProjectDir    string    `json:"project_dir"`
	Config        Config    `json:"config,omitempty"`
	Findings      []Finding `json:"findings"`
	BlockingCount int       `json:"blocking_count"`
	Summary       Summary   `json:"summary"`
	Suggestion    string    `json:"suggestion"`
}

// Config controls the comment checker's behaviour.
type Config struct {
	// AllowedTags lists comment marker tags (e.g. "TODO(admin)") that R001 exempts.
	AllowedTags []string `json:"allowed_tags,omitempty"`
	// AllowedRoots limits file scans to these directories.
	// When empty, Run() defaults to [projectDir] as the sole allowed root.
	AllowedRoots []string `json:"allowed_roots,omitempty"`
	// Files is an explicit file list. Relative paths are resolved against
	// the projectDir passed to Run(). When set, project-dir scanning is skipped.
	Files []string `json:"files,omitempty"`
	// MaxFileSize is the maximum file size in bytes. Files larger than this are skipped.
	// Default is 1 MiB.
	MaxFileSize int64 `json:"max_file_size,omitempty"`
}

// DefaultMaxFileSize is the default maximum file size for comment scanning.
const DefaultMaxFileSize = 1 << 20 // 1 MiB

// Run executes the comment checker against the given project directory.
// It discovers files, applies all rules, and returns a Report.
// It never modifies files on disk.
//
// When cfg.AllowedRoots is empty, the projectDir is used as the sole allowed
// root, preventing access to files outside the project directory.
// Relative paths in cfg.Files are resolved against projectDir.
func Run(projectDir string, cfg Config) (Report, error) {
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = DefaultMaxFileSize
	}

	// Default security boundary: only the project directory is allowed.
	if len(cfg.AllowedRoots) == 0 {
		absProject, err := resolveRoot(projectDir)
		if err != nil {
			return Report{}, fmt.Errorf("resolve project dir: %w", err)
		}
		cfg.AllowedRoots = []string{absProject}
	}

	// Resolve and normalize each file path.
	// Relative paths are resolved against the symlink-resolved projectDir
	// so that macOS /var -> /private/var is handled consistently even for
	// files that do not yet exist on disk.
	absProject, err := resolveRoot(projectDir)
	if err != nil {
		return Report{}, fmt.Errorf("resolve project dir: %w", err)
	}
	files := make([]string, 0, len(cfg.Files))
	for _, f := range cfg.Files {
		resolved := f
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(absProject, resolved)
		}
		resolved = filepath.Clean(resolved)
		files = append(files, resolved)
	}

	if len(files) == 0 {
		var err error
		files, err = discoverFiles(absProject)
		if err != nil {
			return Report{}, fmt.Errorf("discover files: %w", err)
		}
	}

	// Validate path safety (R005)
	for _, f := range files {
		if err := checkPathSafety(f, cfg.AllowedRoots); err != nil {
			return Report{}, &PathSafetyError{Path: f, Err: err}
		}
	}

	report := Report{
		SchemaVersion: SchemaVersion,
		ProjectDir:    projectDir,
		Config:        cfg,
		Findings:      []Finding{},
	}

	// Compile credential regex once.
	credRe := compileCredentialPatterns()
	// Compile tag allowlist patterns.
	allowPatterns := compileAllowPatterns(cfg.AllowedTags)

	for _, filePath := range files {
		info, err := os.Stat(filePath)
		if err != nil {
			report.Summary.SkippedFiles++
			continue
		}
		if info.Size() > cfg.MaxFileSize {
			report.Summary.SkippedFiles++
			continue
		}
		if isBinary(filePath) {
			report.Summary.SkippedFiles++
			continue
		}

		findings, err := checkFile(filePath, allowPatterns, credRe)
		if err != nil {
			// File read error — skip but don't abort.
			report.Summary.SkippedFiles++
			continue
		}
		report.Findings = append(report.Findings, findings...)
		report.Summary.TotalFiles++
	}

	// Compute summary.
	report.Summary.TotalFindings = len(report.Findings)
	for _, f := range report.Findings {
		switch f.Severity {
		case SeverityBlocking:
			report.Summary.BlockingCount++
		case SeverityWarning:
			report.Summary.WarningCount++
		case SeverityInfo:
			report.Summary.InfoCount++
		}
	}
	report.BlockingCount = report.Summary.BlockingCount

	// Build suggestion.
	report.Suggestion = buildSuggestion(report.Summary)

	return report, nil
}

// discoverFiles walks the directory tree and returns all .go and other
// text-source files. This is intentionally limited to avoid scanning
// vendored or generated directories.
func discoverFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// Skip common non-source directories.
			if name == ".git" || name == ".reasonix" || name == "node_modules" ||
				name == "vendor" || name == "artifacts" || name == "cache" ||
				name == ".hyperspace" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".h", ".cpp",
			".hpp", ".rb", ".sh", ".bash", ".yaml", ".yml", ".toml", ".json",
			".md", ".txt", ".html", ".css", ".sql", ".swift", ".kt", ".scala":
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// isBinary checks whether a file appears to be binary by scanning the first
// 512 bytes for a null byte.
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

// checkFile applies comment-checking rules to a single file.
func checkFile(path string, allowPatterns []*regexp.Regexp, credRe *regexp.Regexp) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	scanner := bufio.NewScanner(f)
	lineNum := 0
	var prevLine string

	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		trimmed := strings.TrimSpace(text)

		// Only process lines containing comments.
		comment, ok := extractComment(trimmed)
		if !ok {
			prevLine = trimmed
			continue
		}

		// R002: Empty or placeholder comments.
		if isEmptyComment(comment) {
			findings = append(findings, Finding{
				File:       path,
				Line:       lineNum,
				RuleID:     R002,
				Severity:   SeverityWarning,
				Message:    fmt.Sprintf("Empty or placeholder comment at %s:%d", path, lineNum),
				Suggestion: "Remove the empty comment or add a meaningful description.",
			})
			prevLine = trimmed
			continue
		}

		// R001: Debug markers.
		if finding := checkDebugMarkers(path, lineNum, comment, allowPatterns); finding != nil {
			findings = append(findings, *finding)
			prevLine = trimmed
			continue
		}

		// R004: Credential leak.
		if finding := checkCredentials(path, lineNum, comment, credRe); finding != nil {
			findings = append(findings, *finding)
			prevLine = trimmed
			continue
		}

		// R003: Comment-code similarity (warning only).
		if finding := checkSimilarity(path, lineNum, comment, prevLine); finding != nil {
			findings = append(findings, *finding)
		}

		prevLine = trimmed
	}

	return findings, scanner.Err()
}

// extractComment strips the comment syntax and returns the comment text.
// Returns (comment, true) if the line is a comment line; otherwise ("", false).
func extractComment(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "//")), true
	}
	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "#")), true
	}
	if strings.HasPrefix(trimmed, "--") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "--")), true
	}
	// Block comment start, single-line only for simplicity.
	if strings.HasPrefix(trimmed, "/*") {
		inner := strings.TrimPrefix(trimmed, "/*")
		if idx := strings.Index(inner, "*/"); idx >= 0 {
			return strings.TrimSpace(inner[:idx]), true
		}
		return strings.TrimSpace(inner), true
	}
	return "", false
}

// isEmptyComment checks if a comment is empty or contains only placeholder text.
func isEmptyComment(comment string) bool {
	cleaned := strings.TrimSpace(comment)
	if cleaned == "" {
		return true
	}
	// Bare "TBD" is a placeholder; TODO/FIXME/XXX are handled by R001.
	placeholders := []string{"TBD", "tbd"}
	for _, p := range placeholders {
		if cleaned == p {
			return true
		}
	}
	return false
}

// checkDebugMarkers looks for TODO/FIXME/XXX markers in comment text,
// respecting the allowlist of tagged markers.
func checkDebugMarkers(path string, lineNum int, comment string, allowPatterns []*regexp.Regexp) *Finding {
	// Check allowed patterns first.
	for _, pat := range allowPatterns {
		if pat.MatchString(comment) {
			return nil
		}
	}

	debugPattern := debugMarkerRe
	match := debugPattern.FindString(comment)
	if match == "" {
		return nil
	}

	severity := SeverityInfo
	if match == "FIXME" || match == "BUG" {
		severity = SeverityWarning
	}

	msg := fmt.Sprintf("Comment contains marker %q at %s:%d", match, path, lineNum)
	return &Finding{
		File:       path,
		Line:       lineNum,
		RuleID:     R001,
		Severity:   severity,
		Message:    msg,
		Suggestion: fmt.Sprintf("Resolve the %s marker before committing, or add it to the allowlist.", match),
	}
}

// checkCredentials looks for credential-like patterns in comments.
func checkCredentials(path string, lineNum int, comment string, credRe *regexp.Regexp) *Finding {
	loc := credRe.FindStringSubmatchIndex(comment)
	if loc == nil {
		return nil
	}

	// Try to extract key name: use the captured group (e.g., "password", "api_key")
	// when no text precedes the match.
	keyName := "key"
	if len(loc) >= 4 && loc[2] >= 0 && loc[3] > loc[2] {
		keyName = comment[loc[2]:loc[3]]
	} else if loc[0] > 0 {
		beforeValue := comment[:loc[0]]
		keyName = extractKeyName(beforeValue)
	}

	// Build redacted detail.
	redacted := fmt.Sprintf("%s = *** (redacted)", keyName)

	msg := fmt.Sprintf("Suspected credential in comment at %s:%d", path, lineNum)
	return &Finding{
		File:           path,
		Line:           lineNum,
		RuleID:         R004,
		Severity:       SeverityBlocking,
		Message:        msg,
		RedactedDetail: redacted,
		Suggestion:     "Remove credentials from comments. Use environment variables or a secrets manager.",
	}
}

// extractKeyName tries to find a meaningful key name before a value.
func extractKeyName(before string) string {
	before = strings.TrimSpace(before)
	// Try to extract the last word/identifier.
	parts := strings.Fields(before)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		last = strings.TrimRight(last, "=: \t")
		if last != "" {
			return last
		}
	}
	return "key"
}

// compileCredentialPatterns builds a regex that matches common credential patterns.
// Pre-compiled pattern reused across all checkFile calls.
var debugMarkerRe = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK|WORKAROUND|BUG)\b`)

func compileCredentialPatterns() *regexp.Regexp {
	patterns := []string{
		`(?i)(password|passwd|pwd)\s*[=:]\s*["']?[^\s"']{4,}["']?`,
		`(?i)(api[_-]?key|apikey)\s*[=:]\s*["']?[^\s"']{8,}["']?`,
		`(?i)(secret|token|auth_token|access_token)\s*[=:]\s*["']?[^\s"']{8,}["']?`,
		`\bsk-[a-zA-Z0-9]{20,}\b`,  // OpenAI-style keys
		`\bghp_[a-zA-Z0-9]{36,}\b`, // GitHub PAT
		`\bgho_[a-zA-Z0-9]{36,}\b`, // GitHub OAuth
		`\bghu_[a-zA-Z0-9]{36,}\b`, // GitHub user token
		`\bAKIA[0-9A-Z]{16}\b`,     // AWS access key
	}
	return regexp.MustCompile(strings.Join(patterns, "|"))
}

// compileAllowPatterns converts allowlist tag strings to regex patterns.
func compileAllowPatterns(tags []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(tags))
	for _, tag := range tags {
		// Match the tag at a word boundary start. No trailing boundary
		// because tags may be followed by non-word chars like ':' or '('.
		pat := `\b` + regexp.QuoteMeta(tag)
		re, err := regexp.Compile(pat)
		if err == nil {
			patterns = append(patterns, re)
		}
	}
	return patterns
}

// checkSimilarity detects comments that are overly similar to adjacent code.
// This is a best-effort, warning-only rule.
func checkSimilarity(path string, lineNum int, comment, prevLine string) *Finding {
	if prevLine == "" {
		return nil
	}
	// Simple heuristic: if the comment text appears verbatim in the previous
	// code line, it's likely redundant.
	commentWords := strings.Fields(comment)
	if len(commentWords) < 3 {
		return nil
	}
	// If more than 60% of comment words appear in the previous line, warn.
	matchCount := 0
	for _, w := range commentWords {
		if len(w) < 3 {
			continue
		}
		if strings.Contains(strings.ToLower(prevLine), strings.ToLower(w)) {
			matchCount++
		}
	}
	if matchCount*10 > len(commentWords)*6 {
		msg := fmt.Sprintf("Comment at %s:%d closely resembles adjacent code", path, lineNum)
		return &Finding{
			File:       path,
			Line:       lineNum,
			RuleID:     R003,
			Severity:   SeverityWarning,
			Message:    msg,
			Suggestion: "Rewrite the comment to explain intent rather than restating the code.",
		}
	}
	return nil
}

// PathSafetyError is returned when a file path is outside the allowed root.
type PathSafetyError struct {
	Path string
	Err  error
}

func (e *PathSafetyError) Error() string {
	return fmt.Sprintf("path not allowed: %s: %v", e.Path, e.Err)
}

// checkPathSafety validates that a resolved file path is within any of the
// allowed roots. It resolves symlinks via EvalSymlinks to prevent link-based
// escapes. For non-existent paths it still checks for directory traversal.
func checkPathSafety(path string, allowedRoots []string) error {
	if len(allowedRoots) == 0 {
		return nil
	}

	cleaned := filepath.Clean(path)

	// Resolve symlinks to get the real path. EvalSymlinks follows all
	// intermediate symlinks, so both file-level and directory-level links
	// are handled.
	realPath, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// Path does not exist (or is a broken link). Fall back to
		// Abs to resolve any ../ components and check the directory.
		abs, absErr := filepath.Abs(cleaned)
		if absErr != nil {
			return fmt.Errorf("cannot resolve path %s: %w", path, absErr)
		}
		return checkPathInRoots(abs, allowedRoots, path)
	}

	// EvalSymlinks returns an absolute path when the input is absolute,
	// but we call Abs anyway for consistency.
	abs, err := filepath.Abs(realPath)
	if err != nil {
		return fmt.Errorf("cannot resolve path %s: %w", path, err)
	}
	return checkPathInRoots(abs, allowedRoots, path)
}

// checkPathInRoots checks whether an absolute path is within at least one
// of the allowed roots. Both the path and roots are resolved through
// EvalSymlinks where possible to prevent inconsistent results on platforms
// where parent directories are symlinks (e.g. /var -> /private/var on macOS).
// origPath is used only for error reporting.
func checkPathInRoots(abs string, allowedRoots []string, origPath string) error {
	abs = filepath.Clean(abs)
	for _, root := range allowedRoots {
		absRoot, err := resolveRoot(root)
		if err != nil {
			// Skip unresolvable roots; they cannot be a valid containment parent.
			continue
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return nil
		}
	}
	return fmt.Errorf("file %s is outside the allowed project roots", origPath)
}

// resolveRoot resolves a root directory path to its canonical absolute form.
// It first tries EvalSymlinks (handling /var -> /private/var on macOS),
// then falls back to filepath.Abs.
func resolveRoot(root string) (string, error) {
	cleaned := filepath.Clean(root)
	real, err := filepath.EvalSymlinks(cleaned)
	if err == nil {
		return real, nil
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// HumanString returns a human-readable representation of the report.
func (r Report) HumanString() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Comment Check Report\n"))
	b.WriteString(fmt.Sprintf("  Project: %s\n", r.ProjectDir))
	b.WriteString(fmt.Sprintf("  Schema: v%d\n", r.SchemaVersion))
	b.WriteString(fmt.Sprintf("  Files checked: %d\n", r.Summary.TotalFiles))
	if r.Summary.SkippedFiles > 0 {
		b.WriteString(fmt.Sprintf("  Files skipped: %d\n", r.Summary.SkippedFiles))
	}
	b.WriteString(fmt.Sprintf("  Total findings: %d\n", r.Summary.TotalFindings))
	b.WriteString(fmt.Sprintf("  Blocking: %d\n", r.Summary.BlockingCount))
	b.WriteString(fmt.Sprintf("  Warnings: %d\n", r.Summary.WarningCount))
	b.WriteString(fmt.Sprintf("  Info: %d\n", r.Summary.InfoCount))
	b.WriteString(fmt.Sprintf("  Status: "))
	if r.BlockingCount > 0 {
		b.WriteString(fmt.Sprintf("BLOCKING (%d blocking findings)\n", r.BlockingCount))
	} else if r.Summary.WarningCount > 0 {
		b.WriteString("WARNING\n")
	} else {
		b.WriteString("PASS\n")
	}
	b.WriteString("\n")

	for _, f := range r.Findings {
		b.WriteString(fmt.Sprintf("[%s] [%s] %s\n", f.Severity, f.RuleID, f.Message))
		if f.RedactedDetail != "" {
			b.WriteString(fmt.Sprintf("       %s\n", f.RedactedDetail))
		}
		if f.Suggestion != "" {
			b.WriteString(fmt.Sprintf("       => %s\n", f.Suggestion))
		}
	}

	if r.Suggestion != "" {
		b.WriteString(fmt.Sprintf("\nSuggestion: %s\n", r.Suggestion))
	}

	return b.String()
}

// buildSuggestion generates a human-readable suggestion based on the summary.
func buildSuggestion(s Summary) string {
	if s.BlockingCount > 0 {
		return fmt.Sprintf("Fix %d blocking finding(s) before committing. Review credentials in comments.", s.BlockingCount)
	}
	if s.WarningCount > 0 {
		return "Review warning-level findings. Consider cleaning up empty or redundant comments."
	}
	if s.TotalFindings == 0 {
		return "No comment issues found. Good work keeping comments clean!"
	}
	return "Review the findings before proceeding."
}
