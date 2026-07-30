package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/claude"
	"github.com/mchenziyi/oh-my-reasonix/internal/doctor"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
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

func runDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, _ := loadAssetsFromInvocation()
	result, runErr := doctor.Run(*projectDir, assets)
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return err
		}
	} else {
		result.Render(os.Stdout)
	}
	return runErr
}

func runProfileList(args []string) error {
	flags := flag.NewFlagSet("profile list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := install.ProjectRoot(*projectDir)
	if err != nil {
		return err
	}
	m, err := manifest.Load(install.ManifestPathForDoctor(root))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("OMR manifest not found: %s", install.ManifestPathForDoctor(root))
		}
		return err
	}
	profiles := m.NormalizedProfiles()
	if *jsonOutput {
		configured, categoryByProfile, disabled, configErr := loadProfileConfig(root)
		if configErr != nil {
			return configErr
		}
		output := make([]profileJSON, 0, len(profiles))
		for _, profile := range profiles {
			item := profileJSON{ID: profile.ID, Path: profile.Path, ContentSHA256: profile.ContentSHA256}
			// Source and status
			item.Source = "builtin"
			item.Status = "enabled"
			if disabled[profile.ID] {
				item.Status = "disabled"
			}
			if len(profile.ContentSHA256) >= 8 {
				item.PromptShortHash = profile.ContentSHA256[:8]
			}
			// Model info and SKILL metadata
			applyProfileMetadata(&item, root, profile.Path)
			if agent, ok := configured[profile.ID]; ok {
				applyProfileAgentConfig(&item, root, agent)
			}
			applyProfileRouting(&item, categoryByProfile, disabled)
			output = append(output, item)
		}
		// Append project-only profiles (configured but not installed)
		projectIDs := projectOnlyProfileIDs(profiles, configured)
		for _, id := range projectIDs {
			item := profileJSON{ID: id, Source: "project", Status: "missing"}
			if agent, ok := configured[id]; ok {
				item.Model = agent.Model
				if agent.Model != "" {
					item.EffectiveModel = agent.Model
					item.ModelSource = "project"
				}
			}
			output = append(output, item)
		}
		return json.NewEncoder(os.Stdout).Encode(output)
	}
	configured, categoryByProfile, disabled, configErr := loadProfileConfig(root)
	if configErr != nil {
		return configErr
	}
	for profile := range categoryByProfile {
		sort.Strings(categoryByProfile[profile])
	}
	fmt.Printf("%-16s %-8s %-10s %-18s %s\n", "PROFILE", "SOURCE", "STATUS", "MODEL", "CATEGORIES")
	for _, profile := range profiles {
		source := "builtin"
		status := "enabled"
		if disabled[profile.ID] {
			status = "disabled"
		}
		model := "(default)"
		modelSource := ""
		if agent, ok := configured[profile.ID]; ok && agent.Model != "" {
			model = agent.Model
			modelSource = "(proj)"
		}
		cats := ""
		if categories := categoryByProfile[profile.ID]; len(categories) > 0 {
			cats = strings.Join(categories, ",")
		}
		modelDisplay := model
		if modelSource != "" {
			modelDisplay = model + " " + modelSource
		}
		fmt.Printf("%-16s %-8s %-10s %-18s %s\n", profile.ID, source, status, modelDisplay, cats)
	}
	// Append project-only profiles (configured but not installed)
	projectIDs := projectOnlyProfileIDs(profiles, configured)
	for _, id := range projectIDs {
		model := "(default)"
		if agent, ok := configured[id]; ok && agent.Model != "" {
			model = agent.Model + " (proj)"
		}
		fmt.Printf("%-16s %-8s %-10s %-18s %s\n", id, "project", "missing", model, "")
	}
	return nil
}

func projectRelativePath(projectDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectDir, path)
}
