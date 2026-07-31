package commenthook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/commentchecker"
)

// MaxPayloadSize is the maximum allowed stdin payload size (64 KiB).
const MaxPayloadSize = 64 * 1024

// RunGuard executes the guard logic. It reads a single JSON line from stdin,
// validates it, checks for a direct git commit, and if found runs the comment
// checker.
//
// Returns a GuardResult with the appropriate exit code:
//
//	0 - non-commit, clean, or only warnings/info
//	2 - blocking finding or scan failure
//	1 - invalid payload or event mismatch
func RunGuard(stdin io.Reader, projectDir string) GuardResult {
	limited := io.LimitReader(stdin, MaxPayloadSize)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, MaxPayloadSize), MaxPayloadSize)

	if !scanner.Scan() {
		return GuardResult{
			ExitCode: 1,
			Message:  "empty stdin: expected single-line JSON payload",
		}
	}
	line := scanner.Text()

	if err := scanner.Err(); err != nil {
		return GuardResult{
			ExitCode: 1,
			Message:  "stdin read error",
		}
	}

	var input GuardInput
	if err := json.Unmarshal([]byte(line), &input); err != nil {
		return GuardResult{
			ExitCode: 1,
			Message:  "invalid JSON payload",
		}
	}

	if input.Event != "PreToolUse" {
		return GuardResult{
			ExitCode: 1,
			Message:  fmt.Sprintf("unexpected event %q, expected PreToolUse", input.Event),
		}
	}

	if input.ToolName != "bash" {
		return GuardResult{ExitCode: 0}
	}

	cmd, err := input.GetCommand()
	if err != nil {
		// Bash with invalid/missing/empty command — fail closed.
		return GuardResult{
			ExitCode: 1,
			Message:  "invalid toolArgs",
		}
	}
	if cmd == "" {
		// Bash tool should always have a command; empty means toolArgs is absent.
		return GuardResult{
			ExitCode: 1,
			Message:  "missing toolArgs",
		}
	}

	if !IsGitCommit(cmd) {
		return GuardResult{ExitCode: 0}
	}

	// This is a direct git commit — run the comment checker.
	cfg := commentchecker.Config{}
	report, err := commentchecker.Run(projectDir, cfg)
	if err != nil {
		// Scan failure: fail closed.
		return GuardResult{
			Block:    true,
			ExitCode: 2,
			Message:  "comment check failed",
		}
	}

	if report.BlockingCount > 0 {
		msg := buildBlockingMessage(report)
		return GuardResult{
			Block:    true,
			ExitCode: 2,
			Message:  msg,
		}
	}

	// Clean, warning-only, or info-only — do not block.
	if report.Summary.TotalFindings > 0 {
		warnings := report.Summary.WarningCount + report.Summary.InfoCount
		msg := fmt.Sprintf("%d non-blocking finding(s) (%d warning(s), %d info) - review recommended",
			warnings, report.Summary.WarningCount, report.Summary.InfoCount)
		return GuardResult{
			ExitCode: 0,
			Message:  msg,
		}
	}

	return GuardResult{ExitCode: 0}
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
