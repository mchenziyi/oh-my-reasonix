package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/commenthook"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

func runHook(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("hook requires a subcommand: doctor|comment-check")
	}

	switch args[0] {
	case "doctor":
		return runHookDoctor(args[1:])
	case "comment-check":
		return runHookCommentCheck(args[1:])
	default:
		return fmt.Errorf("unknown hook subcommand %q; expected doctor|comment-check", args[0])
	}
}

// --- Hook Doctor (existing) ---

func runHookDoctor(args []string) error {
	flags := flag.NewFlagSet("hook doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	homeDir := flags.String("home-dir", "", "Reasonix home directory (sets REASONIX_HOME)")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	if *homeDir != "" {
		runner.Env = append(runner.Env, "REASONIX_HOME="+*homeDir)
	}
	ctx := context.Background()
	listResult, listErr := runner.HookList(ctx)
	if listErr != nil {
		return fmt.Errorf("hook doctor: %w", listErr)
	}
	statusResult := runner.HookStatus(ctx)

	if *jsonOutput {
		type hookDoctorOutput struct {
			List   reasonix.HookListOutput   `json:"list"`
			Status reasonix.HookStatusOutput `json:"status"`
		}
		return writeJSONOutput(hookDoctorOutput{List: listResult, Status: statusResult})
	}
	if len(listResult.Hooks) == 0 {
		fmt.Println("No hooks found")
	}
	fmt.Printf("%-20s %-16s %-10s %s\n", "EVENT", "MATCH", "SCOPE", "STATUS")
	for _, h := range listResult.Hooks {
		fmt.Printf("%-20s %-16s %-10s %s\n", h.Event, h.Match, h.Scope, h.Status)
	}
	if statusResult.Unavailable {
		fmt.Printf("STATUS: unavailable — %s\n", statusResult.Error)
	} else {
		fmt.Printf("STATUS: trusted_project=%t project_defines=%t\n",
			statusResult.TrustedProject, statusResult.ProjectDefines)
		for _, source := range statusResult.Sources {
			fmt.Printf("SOURCE: scope=%s status=%s hooks=%d\n", source.Scope, source.Status, source.HookCount)
		}
	}
	return nil
}

// --- Hook Comment Check ---

func runHookCommentCheck(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("comment-check requires a subcommand: enable|status|disable|guard|logs")
	}

	switch args[0] {
	case "enable":
		return runHookCommentCheckEnable(args[1:])
	case "status":
		return runHookCommentCheckStatus(args[1:])
	case "disable":
		return runHookCommentCheckDisable(args[1:])
	case "guard":
		return runHookCommentCheckGuard(args[1:])
	case "logs":
		return runHookCommentCheckLogs(args[1:])
	default:
		return fmt.Errorf("unknown comment-check subcommand %q; expected enable|status|disable|guard|logs", args[0])
	}
}

// --- logs ---

type auditLogsOutput struct {
	SchemaVersion int                      `json:"schema_version"`
	ProjectDir    string                   `json:"project_dir"`
	Entries       int                      `json:"entries"`
	Summary       map[string]int           `json:"summary"`
	Logs          []commenthook.AuditEntry `json:"logs,omitempty"`
	Cleared       bool                     `json:"cleared,omitempty"`
	DryRun        bool                     `json:"dry_run,omitempty"`
}

func runHookCommentCheckLogs(args []string) error {
	flags := flag.NewFlagSet("hook comment-check logs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	clear := flags.Bool("clear", false, "clear the audit log")
	dryRun := flags.Bool("dry-run", false, "preview the clear without writing")
	if err := flags.Parse(args); err != nil {
		return err
	}

	store, err := commenthook.NewAuditStore(*projectDir)
	if err != nil {
		return err
	}

	out := auditLogsOutput{SchemaVersion: 1, ProjectDir: *projectDir, Summary: map[string]int{}, Logs: []commenthook.AuditEntry{}}

	if *clear {
		if *dryRun {
			if _, err := store.Clear(commenthook.DryRun); err != nil {
				return err
			}
			out.DryRun = true
		} else {
			if _, err := store.Clear(commenthook.RealClear); err != nil {
				return err
			}
			out.Cleared = true
		}
		if *jsonOutput {
			return writePrettyJSONOutput(out)
		}
		if out.Cleared {
			fmt.Println("Comment Checker audit log cleared.")
		} else {
			fmt.Println("DRY-RUN: audit log would be cleared.")
		}
		return nil
	}

	entries, err := store.List()
	if err != nil {
		return err
	}
	out.Entries = len(entries)
	out.Logs = entries
	for _, e := range entries {
		out.Summary[e.Decision]++
	}
	if *jsonOutput {
		return writePrettyJSONOutput(out)
	}
	fmt.Printf("Comment Checker audit log: %d entries\n", out.Entries)
	for decision, count := range out.Summary {
		fmt.Printf("  %-14s %d\n", decision, count)
	}
	for _, e := range entries {
		fmt.Printf("  %s %s exit=%d rules=%v\n", e.Time, e.Decision, e.ExitCode, e.RuleCounts)
	}
	return nil
}

// --- enable ---

func runHookCommentCheckEnable(args []string) error {
	flags := flag.NewFlagSet("hook comment-check enable", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	if err := flags.Parse(args); err != nil {
		return err
	}

	omrPath, resolveErr := commenthook.ResolveOmrPath()

	report, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir:      *projectDir,
		DryRun:          *dryRun,
		OmrNotAvailable: resolveErr != nil,
		OmrCommand:      omrPath,
	})
	renderHookReport(report)
	if err != nil {
		return err
	}

	if report.Written {
		fmt.Println("Comment Checker Hook enabled. Restart Reasonix for the change to take effect.")
	}
	return nil
}

// --- status ---

type hookStatusOutput struct {
	SchemaVersion    int      `json:"schema_version"`
	Enabled          bool     `json:"enabled"`
	Owned            bool     `json:"owned"`
	SettingsPath     string   `json:"settings_path"`
	Event            string   `json:"event"`
	Match            string   `json:"match"`
	CommandAvailable bool     `json:"command_available"`
	ReasonixVisible  bool     `json:"reasonix_visible"`
	Issues           []string `json:"issues"`
}

func runHookCommentCheckStatus(args []string) error {
	flags := flag.NewFlagSet("hook comment-check status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := commenthook.ResolveProjectRoot(*projectDir)
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}

	settingsPath := commenthook.SettingsPath(root)
	issues := []string{}

	// Read and parse settings. Only os.IsNotExist is treated as empty.
	// Other read errors are reported as issues.
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			issues = append(issues, "cannot read settings: "+err.Error())
		}
		// raw is nil, ParseSettings handles nil as empty.
	}
	parsed, parseErr := commenthook.ParseSettings(raw)
	if parseErr != nil {
		issues = append(issues, "cannot parse settings: "+parseErr.Error())
	}

	omrPath, resolveErr := commenthook.ResolveOmrPath()
	enabled := false
	owned := false
	legacy := false
	if parseErr == nil && parsed != nil {
		entries := parsed.Hooks["PreToolUse"]
		markerCount := 0
		for _, re := range entries {
			if re.HasOMRDescription() {
				markerCount++
			}
			if re.IsOMROwnedFor(omrPath) {
				enabled = true
				owned = true
				if entry, ok := re.Entry(); ok && entry.Command == commenthook.OMRCommandLegacy {
					legacy = true
				}
			} else if re.HasOMRDescription() {
				issues = append(issues, "OMR entry differs from the expected command or fields; manual resolution required")
			}
		}
		if markerCount > 1 {
			enabled = false
			owned = false
			issues = append(issues, "multiple OMR Hook entries found; manual resolution required")
		}
	}
	if legacy && owned {
		issues = append(issues, "legacy PATH-dependent Hook command found; rerun enable to migrate to an absolute executable path")
	}

	omrAvailable := resolveErr == nil
	if !omrAvailable {
		issues = append(issues, "stable omr executable path cannot be resolved; hook command will not execute")
	}

	out := hookStatusOutput{
		SchemaVersion:    1,
		Enabled:          enabled,
		Owned:            owned,
		SettingsPath:     settingsPath,
		Event:            "PreToolUse",
		Match:            "bash",
		CommandAvailable: omrAvailable,
		Issues:           issues,
	}

	// Check visibility via Reasonix (best-effort).
	runner := reasonix.Runner{ProjectDir: root}
	ctx := context.Background()
	listResult, listErr := runner.HookList(ctx)
	if listErr == nil {
		for _, h := range listResult.Hooks {
			if h.Event == "PreToolUse" && h.Match == "bash" && h.Scope == "project" &&
				(h.Status == "active" || h.Status == "enabled") {
				out.ReasonixVisible = true
				break
			}
		}
	}

	if *jsonOutput {
		return writePrettyJSONOutput(out)
	}

	fmt.Printf("Comment Checker Hook Status\n")
	fmt.Printf("  Enabled: %t\n", out.Enabled)
	fmt.Printf("  OMR-owned: %t\n", out.Owned)
	fmt.Printf("  Settings: %s\n", out.SettingsPath)
	fmt.Printf("  Event: %s\n", out.Event)
	fmt.Printf("  Match: %s\n", out.Match)
	fmt.Printf("  omr executable available: %t\n", out.CommandAvailable)
	fmt.Printf("  Reasonix visible: %t\n", out.ReasonixVisible)
	if len(issues) > 0 {
		fmt.Printf("  Issues:\n")
		for _, iss := range issues {
			fmt.Printf("    - %s\n", iss)
		}
	}
	return nil
}

// --- disable ---

func runHookCommentCheckDisable(args []string) error {
	flags := flag.NewFlagSet("hook comment-check disable", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	if err := flags.Parse(args); err != nil {
		return err
	}

	omrPath, _ := commenthook.ResolveOmrPath()
	report, err := commenthook.DisableHook(commenthook.TransactionOptions{
		ProjectDir: *projectDir,
		DryRun:     *dryRun,
		OmrCommand: omrPath,
	})
	renderHookReport(report)
	if err != nil {
		return err
	}

	if report.Written {
		fmt.Println("Comment Checker Hook disabled. Restart Reasonix for the change to take effect.")
	}
	return nil
}

// --- guard ---

func runHookCommentCheckGuard(args []string) error {
	flags := flag.NewFlagSet("hook comment-check guard", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := commenthook.ResolveProjectRoot(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "guard: resolve project dir: %v\n", err)
		os.Exit(2)
	}

	result := commenthook.RunGuardFromStdin(root)

	if result.ExitCode != 0 && result.Message != "" {
		fmt.Fprintln(os.Stderr, result.Message)
	}

	os.Exit(result.ExitCode)
	return nil // unreachable
}

// --- helpers ---

func renderHookReport(r commenthook.HookReport) {
	for _, change := range r.Changes {
		fmt.Println(change)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintln(os.Stderr, "WARNING:", warning)
	}
	for _, conflict := range r.Conflicts {
		fmt.Fprintln(os.Stderr, "CONFLICT:", conflict)
	}
	for _, err := range r.Errors {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	if r.NoOp {
		fmt.Println("NOOP: nothing to do")
	}
}
