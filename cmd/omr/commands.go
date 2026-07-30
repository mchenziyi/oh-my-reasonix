package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/claude"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
)

func runInstall(args []string, upgrade bool) error {
	flags := flag.NewFlagSet(map[bool]string{true: "upgrade", false: "init"}[upgrade], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	compose := flags.Bool("compose-prompt", false, "explicitly compose an existing user Prompt")
	allowPersist := flags.Bool("allow-persist-user-prompt", false, "confirm that a non-empty User Prompt may be persisted")
	acceptBase := flags.Bool("accept-reasonix-base-update", false, "accept a changed Reasonix base Prompt during upgrade")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, err := loadAssetsFromInvocation()
	if err != nil {
		return err
	}
	report, runErr := install.Init(install.Options{
		ProjectDir:               *projectDir,
		DryRun:                   *dryRun,
		ComposePrompt:            *compose,
		AllowPersistUserPrompt:   *allowPersist,
		AcceptReasonixBaseUpdate: *acceptBase,
		Upgrade:                  upgrade,
		Assets:                   assets,
	})
	report.Render(os.Stdout)
	return runErr
}

func runClaude(args []string) error {
	if len(args) == 0 {
		return errors.New("claude requires import, rules, skills, agents, commands, mcp, or hooks")
	}
	sub := args[0]

	flags := flag.NewFlagSet("claude "+sub, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	dryRun := flags.Bool("dry-run", false, "show what would be imported")
	force := flags.Bool("force", false, "overwrite existing files")
	jsonOut := flags.Bool("json", false, "output report as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	opts := claude.Options{
		ProjectDir: *projectDir,
		DryRun:     *dryRun,
		Force:      *force,
	}

	var report claude.Report
	switch sub {
	case "rules":
		report = claude.ImportRules(opts)
	case "skills":
		report = claude.ImportSkills(opts)
	case "agents":
		report = claude.ImportAgents(opts)
	case "commands":
		report = claude.ImportCommands(opts)
	case "mcp":
		report = claude.ImportMCP(opts)
	case "hooks":
		report = claude.ImportHooks(opts)
	case "import":
		report = claude.ImportAll(opts)
	default:
		return fmt.Errorf("unknown claude subcommand %q (use: import, rules, skills, agents, commands, mcp, hooks)", sub)
	}

	if *jsonOut {
		report.RenderJSON(os.Stdout)
	} else {
		report.Render(os.Stdout)
	}
	if len(report.Errors) > 0 {
		return fmt.Errorf("claude %s failed", sub)
	}
	if len(report.Conflicts) > 0 && !report.Written {
		return fmt.Errorf("claude %s blocked by conflicts", sub)
	}
	return nil
}

func runUninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	report, runErr := install.Uninstall(install.Options{ProjectDir: *projectDir, DryRun: *dryRun})
	report.Render(os.Stdout)
	return runErr
}
