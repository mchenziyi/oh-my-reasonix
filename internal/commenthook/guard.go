package commenthook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/commentchecker"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

// MaxPayloadSize is the maximum allowed stdin payload size (64 KiB).
const MaxPayloadSize = 64 * 1024

// RunGuard executes the guard logic. It reads a single JSON line from stdin,
// validates it, checks for a direct git commit, and if found runs the comment
// checker. Every decision is appended to the project audit log; an audit
// write failure fails closed so a broken audit trail is never silent.
//
// Returns a GuardResult with the appropriate exit code:
//
//	0 - non-commit, clean, or only warnings/info
//	2 - blocking finding or scan failure
//	1 - invalid payload or event mismatch
func RunGuard(stdin io.Reader, projectDir string) GuardResult {
	started := time.Now()
	store, auditErr := NewAuditStore(projectDir)
	audit := func(decision string, exitCode int, ruleCounts map[string]int, triggered string, message string) GuardResult {
		entry := AuditEntry{
			Time:          time.Now().UTC().Format(time.RFC3339Nano),
			Event:         "PreToolUse",
			Decision:      decision,
			RuleCounts:    ruleCounts,
			ExitCode:      exitCode,
			DurationMs:    time.Since(started).Milliseconds(),
			OMRVersion:    manifest.Version,
			TriggeredRule: triggered,
		}
		if auditErr == nil {
			if err := store.Append(entry); err != nil {
				// Fail closed: a broken audit trail must never look like a
				// silent success. Preserve a blocking decision's code; a
				// pass decision degrades to an explicit failure.
				if exitCode == 2 {
					return GuardResult{Block: true, ExitCode: 2, Message: "comment check failed; audit log unavailable: " + sanitizeMessage(err.Error())}
				}
				return GuardResult{Block: true, ExitCode: 1, Message: "audit log unavailable: " + sanitizeMessage(err.Error())}
			}
		}
		return GuardResult{Block: exitCode == 2, ExitCode: exitCode, Message: message}
	}

	limited := io.LimitReader(stdin, MaxPayloadSize)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, MaxPayloadSize), MaxPayloadSize)

	if !scanner.Scan() {
		return audit(DecisionParseFailure, 1, nil, "empty_stdin", "empty stdin: expected single-line JSON payload")
	}
	line := scanner.Text()

	if err := scanner.Err(); err != nil {
		return audit(DecisionParseFailure, 1, nil, "stdin_read_error", "stdin read error")
	}

	var input GuardInput
	if err := json.Unmarshal([]byte(line), &input); err != nil {
		return audit(DecisionParseFailure, 1, nil, "invalid_json", "invalid JSON payload")
	}

	if input.Event != "PreToolUse" {
		return audit(DecisionParseFailure, 1, nil, "unexpected_event", fmt.Sprintf("unexpected event %q, expected PreToolUse", input.Event))
	}

	if input.ToolName != "bash" {
		return audit(DecisionPass, 0, nil, "", "")
	}

	cmd, err := input.GetCommand()
	if err != nil {
		// Bash with invalid/missing/empty command — fail closed.
		return audit(DecisionParseFailure, 1, nil, "invalid_tool_args", "invalid toolArgs")
	}
	if cmd == "" {
		// Bash tool should always have a command; empty means toolArgs is absent.
		return audit(DecisionParseFailure, 1, nil, "missing_tool_args", "missing toolArgs")
	}

	if !IsGitCommit(cmd) {
		return audit(DecisionPass, 0, nil, "", "")
	}

	// This is a direct git commit — run the comment checker.
	cfg := commentchecker.Config{}
	report, err := commentchecker.Run(projectDir, cfg)
	if err != nil {
		// Scan failure: fail closed.
		return audit(DecisionBlocking, 2, nil, "scan_failure", "comment check failed")
	}

	if report.BlockingCount > 0 {
		msg := buildBlockingMessage(report)
		return audit(DecisionBlocking, 2, ruleCounts(report), firstBlockingRule(report), msg)
	}

	// Clean, warning-only, or info-only — do not block.
	if report.Summary.TotalFindings > 0 {
		warnings := report.Summary.WarningCount + report.Summary.InfoCount
		msg := fmt.Sprintf("%d non-blocking finding(s) (%d warning(s), %d info) - review recommended",
			warnings, report.Summary.WarningCount, report.Summary.InfoCount)
		return audit(DecisionWarning, 0, ruleCounts(report), firstWarningRule(report), msg)
	}

	return audit(DecisionPass, 0, ruleCounts(report), "", "")
}

// ruleCounts aggregates sanitized finding counts per severity.
func ruleCounts(report commentchecker.Report) map[string]int {
	counts := map[string]int{
		"blocking": report.BlockingCount,
		"warning":  report.Summary.WarningCount,
		"info":     report.Summary.InfoCount,
	}
	return counts
}

// firstBlockingRule returns the rule id of the first blocking finding, if any.
func firstBlockingRule(report commentchecker.Report) string {
	for _, f := range report.Findings {
		if f.Severity == commentchecker.SeverityBlocking {
			return string(f.RuleID)
		}
	}
	return ""
}

// firstWarningRule returns the rule id of the first warning finding, if any.
func firstWarningRule(report commentchecker.Report) string {
	for _, f := range report.Findings {
		if f.Severity == commentchecker.SeverityWarning {
			return string(f.RuleID)
		}
	}
	return ""
}

// buildBlockingMessage creates a sanitized, redacted summary of blocking
// findings. It does NOT output full toolArgs, command bodies, or raw
// credentials.
func buildBlockingMessage(report commentchecker.Report) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d blocking finding(s) found", report.BlockingCount))
	if report.BlockingCount > 0 && len(report.Findings) > 0 {
		count := 0
		for _, f := range report.Findings {
			if f.Severity == commentchecker.SeverityBlocking {
				if count >= 3 {
					b.WriteString(fmt.Sprintf("\n  ... and %d more", report.BlockingCount-3))
					break
				}
				if f.RedactedDetail != "" {
					b.WriteString(fmt.Sprintf("\n  %s:%d - %s (%s)", f.File, f.Line, f.Message, f.RedactedDetail))
				} else {
					b.WriteString(fmt.Sprintf("\n  %s:%d - %s", f.File, f.Line, f.Message))
				}
				count++
			}
		}
	}
	b.WriteString("\nFix blocking findings before committing.")
	return b.String()
}

// RunGuardFromStdin is a convenience wrapper that reads from os.Stdin.
func RunGuardFromStdin(projectDir string) GuardResult {
	return RunGuard(os.Stdin, projectDir)
}
