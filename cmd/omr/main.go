package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/cacheguard"
	"github.com/mchenziyi/oh-my-reasonix/internal/claude"
	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/doctor"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/qualitybench"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

var version = "1.1.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = runInstall(os.Args[2:], false)
	case "upgrade":
		err = runInstall(os.Args[2:], true)
	case "uninstall":
		err = runUninstall(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "config":
		err = runConfig(os.Args[2:])
	case "profile":
		err = runProfile(os.Args[2:])
	case "session":
		err = runSession(os.Args[2:])
	case "benchmark":
		err = runBenchmark(os.Args[2:])
	case "claude":
		err = runClaude(os.Args[2:])
	case "hook":
		err = runHook(os.Args[2:])
	case "task":
		err = runTask(os.Args[2:])
	case "version":
		err = runVersion(os.Args[2:])
	case "run":
		err = runRun(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "omr:", err)
		os.Exit(1)
	}
}

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
	if err := flags.Parse(args[1:]); err != nil {
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

func runConfig(args []string) error {
	if len(args) == 0 || (args[0] != "validate" && args[0] != "schema" && args[0] != "migrate") {
		return errors.New("config requires validate, schema, or migrate")
	}
	if args[0] == "migrate" {
		return runConfigMigrate(args[1:])
	}
	if args[0] == "schema" {
		return writeOMRConfigSchema()
	}
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	configPath := flags.String("config", "", "OMR config path (TOML or JSONC)")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = omrconfig.FindConfig(*projectDir)
	}
	cfg, err := omrconfig.Load(path)
	if err != nil {
		// Missing config is not an error — it means the project is not yet configured.
		if os.IsNotExist(err) {
			if *jsonOutput {
				_ = json.NewEncoder(os.Stdout).Encode(struct {
					Path       string `json:"path"`
					Valid      bool   `json:"valid"`
					Configured bool   `json:"configured"`
				}{Path: path, Valid: true, Configured: false})
			} else {
				fmt.Printf("No OMR config found at %s (project not yet configured)\n", path)
			}
			return nil
		}
		if *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Path       string   `json:"path"`
				Valid      bool     `json:"valid"`
				Configured bool     `json:"configured"`
				Error      string   `json:"error"`
				Errors     []string `json:"errors"`
			}{Path: path, Valid: false, Configured: true, Error: err.Error(), Errors: []string{err.Error()}})
		}
		return err
	}
	if conflicts := cfg.DisabledRoutingConflicts(); len(conflicts) > 0 {
		messages := make([]string, 0, len(conflicts))
		for _, category := range conflicts {
			messages = append(messages, fmt.Sprintf("OMR category %q routes to disabled Profile %q", category, cfg.Categories[category]))
		}
		err = errors.New(strings.Join(messages, "; "))
		if *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Path       string   `json:"path"`
				Valid      bool     `json:"valid"`
				Configured bool     `json:"configured"`
				Error      string   `json:"error"`
				Errors     []string `json:"errors"`
			}{Path: path, Valid: false, Configured: true, Error: err.Error(), Errors: messages})
		}
		return err
	}
	// Category diagnostic: check each category routes to an existing profile
	var categoryDiags []string
	// Known profiles from built-in set
	knownProfiles := map[string]bool{
		"omr-explore": true, "omr-research": true, "omr-debug": true,
		"omr-planner": true, "omr-frontend": true,
	}
	// Also check agent configs
	for profile := range cfg.Agents {
		knownProfiles[profile] = true
	}
	for cat, profile := range cfg.Categories {
		if !knownProfiles[profile] {
			categoryDiags = append(categoryDiags, fmt.Sprintf("category %q routes to unknown profile %q", cat, profile))
		}
	}
	sort.Strings(categoryDiags)
	mcpDiags := omrconfig.DiagnoseMCP(cfg)
	for _, diagnostic := range mcpDiags {
		if diagnostic.Enabled && (diagnostic.Availability != "ready" || diagnostic.Compatibility != "compatible") {
			categoryDiags = append(categoryDiags, fmt.Sprintf("MCP server %q is %s", diagnostic.Server, diagnostic.Summary()))
		}
	}
	if promptErrors := validatePromptFiles(cfg, *projectDir); len(promptErrors) > 0 {
		err = errors.New(strings.Join(promptErrors, "; "))
		if *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Path       string   `json:"path"`
				Valid      bool     `json:"valid"`
				Configured bool     `json:"configured"`
				Error      string   `json:"error"`
				Errors     []string `json:"errors"`
			}{Path: path, Valid: false, Configured: true, Error: err.Error(), Errors: promptErrors})
		}
		return err
	}
	if *jsonOutput {
		output := struct {
			Path             string                           `json:"path"`
			Valid            bool                             `json:"valid"`
			Configured       bool                             `json:"configured"`
			Agents           map[string]omrconfig.AgentConfig `json:"agents"`
			Categories       map[string]string                `json:"categories"`
			Concurrency      int                              `json:"concurrency"`
			MaxCost          float64                          `json:"max_cost"`
			DisabledProfiles []string                         `json:"disabled_profiles"`
			MCP              []omrconfig.MCPDiagnostic        `json:"mcp"`
			Warnings         []string                         `json:"warnings,omitempty"`
		}{Path: path, Valid: true, Configured: true, Agents: cfg.Agents, Categories: cfg.Categories, Concurrency: cfg.Concurrency, MaxCost: cfg.MaxCost, DisabledProfiles: cfg.DisabledProfiles, MCP: mcpDiags, Warnings: categoryDiags}
		_ = json.NewEncoder(os.Stdout).Encode(output)
		return nil
	}
	fmt.Printf("OMR config valid: %s\n", path)
	for _, diag := range categoryDiags {
		fmt.Printf("  WARNING: %s\n", diag)
	}
	if cfg.Concurrency > 0 {
		fmt.Printf("  concurrency: %d\n", cfg.Concurrency)
	}
	if cfg.MaxCost > 0 {
		fmt.Printf("  max_cost: %.4f\n", cfg.MaxCost)
	}
	if len(cfg.Categories) > 0 {
		fmt.Printf("  categories: %d\n", len(cfg.Categories))
	}
	for _, diagnostic := range mcpDiags {
		fmt.Printf("  mcp.%s: %s\n", diagnostic.Server, diagnostic.Summary())
	}
	return nil
}

func runConfigMigrate(args []string) error {
	flags := flag.NewFlagSet("config migrate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	doWrite := flags.Bool("write", false, "execute the migration (default: dry-run)")
	doForce := flags.Bool("force", false, "overwrite existing JSONC destination")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sourcePath, destPath := omrconfig.DefaultConfigPaths(*projectDir)
	plan := omrconfig.PlanMigration(sourcePath, destPath)

	if !*doWrite {
		// Dry-run mode: print plan
		fmt.Printf("OMR config migration plan\n")
		fmt.Printf("  Source: %s\n", sourcePath)
		fmt.Printf("  Dest:   %s\n", destPath)
		fmt.Printf("  Backup: %s\n", sourcePath+".bak")
		if !plan.SourceExists {
			fmt.Printf("  Status: source not found\n")
			return fmt.Errorf("source config not found: %s", sourcePath)
		}
		if plan.AlreadyDone {
			fmt.Printf("  Status: already up-to-date (no migration needed)\n")
			return nil
		}
		if plan.Conflict != "" {
			fmt.Printf("  Status: conflict — %s\n", plan.Conflict)
			return fmt.Errorf("migration blocked: %s (use --force to overwrite)", plan.Conflict)
		}
		fmt.Printf("  Status: ready to migrate (use --write to apply)\n")
		return nil
	}

	if err := omrconfig.ExecuteMigration(sourcePath, destPath, *doForce); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	fmt.Printf("Migrated: %s → %s\n", sourcePath, destPath)
	fmt.Printf("  Backup: %s\n", sourcePath+".bak")
	return nil
}

func validatePromptFiles(cfg omrconfig.Config, projectDir string) []string {
	profiles := make([]string, 0, len(cfg.Agents))
	for profile := range cfg.Agents {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	errorsFound := []string{}
	for _, profile := range profiles {
		promptFile := cfg.Agents[profile].PromptFile
		if promptFile == "" {
			continue
		}
		path := promptFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			errorsFound = append(errorsFound, fmt.Sprintf("Prompt file for Profile %q not found: %s", profile, promptFile))
		}
	}
	return errorsFound
}

func writeOMRConfigSchema() error {
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"quality": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
				"fixtures": map[string]string{"type": "string"}, "min_qualified_rate": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "max_cost": map[string]any{"type": "number", "minimum": 0},
			}},
			"runtime": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
				"metrics_dir": map[string]string{"type": "string"}, "model": map[string]string{"type": "string"}, "max_steps": map[string]any{"type": "integer", "minimum": 0}, "concurrency": map[string]any{"type": "integer", "minimum": 0}, "timeout": map[string]string{"type": "string"},
			}},
			"agent": map[string]any{"type": "object", "additionalProperties": map[string]any{
				"type": "object", "additionalProperties": false, "properties": map[string]any{
					"model": map[string]string{"type": "string"}, "prompt_file": map[string]string{"type": "string"}, "read_only": map[string]any{"type": "boolean"},
				},
			}, "propertyNames": map[string]any{"pattern": "^[a-z][a-z0-9-]*$"}},
			"routing":  map[string]any{"type": "object", "additionalProperties": map[string]string{"type": "string"}, "propertyNames": map[string]any{"pattern": "^[a-z][a-z0-9-]*$"}},
			"profiles": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"disabled": map[string]string{"type": "string"}}},
			"mcp": map[string]any{"type": "object", "additionalProperties": map[string]any{
				"type": "object", "additionalProperties": false, "properties": map[string]any{
					"transport":    map[string]any{"type": "string", "enum": []string{"stdio", "http", "sse"}},
					"command":      map[string]string{"type": "string"},
					"args":         map[string]any{"type": "array", "items": map[string]string{"type": "string"}},
					"url":          map[string]any{"type": "string", "pattern": "^https?://"},
					"capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9-]*$"}},
					"enabled":      map[string]any{"type": "boolean"},
					"env":          map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": "^[A-Za-z_][A-Za-z0-9_]*$"}},
				},
				"allOf": []any{
					map[string]any{"if": map[string]any{"properties": map[string]any{"transport": map[string]any{"const": "stdio"}}}, "then": map[string]any{"required": []string{"command"}}},
					map[string]any{"if": map[string]any{"required": []string{"transport"}, "properties": map[string]any{"transport": map[string]any{"enum": []string{"http", "sse"}}}}, "then": map[string]any{"required": []string{"url"}}},
				},
			}, "propertyNames": map[string]any{"pattern": "^[a-z][a-z0-9-]{0,63}$"}},
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(schema)
}

func runProfile(args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("profile requires list")
	}
	flags := flag.NewFlagSet("profile list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args[1:]); err != nil {
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
		type profileJSON struct {
			ID               string   `json:"id"`
			Path             string   `json:"path"`
			ContentSHA256    string   `json:"content_sha256"`
			Model            string   `json:"model,omitempty"`
			PromptFile       string   `json:"prompt_file,omitempty"`
			PromptFileExists *bool    `json:"prompt_file_exists,omitempty"`
			ReadOnly         *bool    `json:"read_only,omitempty"`
			Categories       []string `json:"categories,omitempty"`
			Disabled         bool     `json:"disabled,omitempty"`
			Description      string   `json:"description,omitempty"`
			ReadOnlyBool     bool     `json:"read_only_bool"`
			AllowedTools     []string `json:"allowed_tools,omitempty"`
			InputTypes       []string `json:"input_types,omitempty"`
			OutputSections   []string `json:"output_sections,omitempty"`
			Source           string   `json:"source"`
			Status           string   `json:"status"`
			EffectiveModel   string   `json:"effective_model,omitempty"`
			ModelSource      string   `json:"model_source,omitempty"`
			PromptShortHash  string   `json:"prompt_short_hash,omitempty"`
		}
		configured := map[string]omrconfig.AgentConfig{}
		categoryByProfile := map[string][]string{}
		disabled := map[string]bool{}
		configPath := omrconfig.FindConfig(root)
		if _, statErr := os.Stat(configPath); statErr == nil {
			cfg, configErr := omrconfig.Load(configPath)
			if configErr != nil {
				return configErr
			}
			configured = cfg.Agents
			for category, profile := range cfg.Categories {
				categoryByProfile[profile] = append(categoryByProfile[profile], category)
			}
			for _, profile := range cfg.DisabledProfiles {
				disabled[profile] = true
			}
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
			// Model info
			// Read and parse SKILL.md for metadata
			skillPath := install.ProfilePath(root, profile.Path)
			if data, readErr := os.ReadFile(skillPath); readErr == nil {
				if meta, parseErr := manifest.ParseProfileMeta(data); parseErr == nil {
					item.Description = meta.Description
					item.ReadOnlyBool = meta.ReadOnly
					item.AllowedTools = meta.AllowedTools
					item.InputTypes = meta.InputTypes
					item.OutputSections = meta.OutputSections
				}
			}
			if agent, ok := configured[profile.ID]; ok {
				item.Model, item.PromptFile, item.ReadOnly = agent.Model, agent.PromptFile, agent.ReadOnly
				if agent.Model != "" {
					item.EffectiveModel = agent.Model
					item.ModelSource = "project"
				}
				if agent.PromptFile != "" {
					promptPath := agent.PromptFile
					if !filepath.IsAbs(promptPath) {
						promptPath = filepath.Join(root, promptPath)
					}
					exists := false
					if info, statErr := os.Stat(promptPath); statErr == nil && !info.IsDir() {
						exists = true
					}
					item.PromptFileExists = &exists
				}
			}
			item.Categories = append([]string(nil), categoryByProfile[profile.ID]...)
			sort.Strings(item.Categories)
			item.Disabled = disabled[profile.ID]
			output = append(output, item)
		}
		// Append project-only profiles (configured but not installed)
		manifestIDs := make(map[string]bool, len(profiles))
		for _, p := range profiles {
			manifestIDs[p.ID] = true
		}
		var projectIDs []string
		for id := range configured {
			if !manifestIDs[id] {
				projectIDs = append(projectIDs, id)
			}
		}
		sort.Strings(projectIDs)
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
	categoryByProfile := map[string][]string{}
	disabled := map[string]bool{}
	configured := map[string]omrconfig.AgentConfig{}
	configPath := omrconfig.FindConfig(root)
	if _, statErr := os.Stat(configPath); statErr == nil {
		cfg, configErr := omrconfig.Load(configPath)
		if configErr != nil {
			return configErr
		}
		configured = cfg.Agents
		for category, profile := range cfg.Categories {
			categoryByProfile[profile] = append(categoryByProfile[profile], category)
		}
		for profile := range categoryByProfile {
			sort.Strings(categoryByProfile[profile])
		}
		for _, profile := range cfg.DisabledProfiles {
			disabled[profile] = true
		}
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
	manifestIDs := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		manifestIDs[p.ID] = true
	}
	var projectIDs []string
	for id := range configured {
		if !manifestIDs[id] {
			projectIDs = append(projectIDs, id)
		}
	}
	sort.Strings(projectIDs)
	for _, id := range projectIDs {
		model := "(default)"
		if agent, ok := configured[id]; ok && agent.Model != "" {
			model = agent.Model + " (proj)"
		}
		fmt.Printf("%-16s %-8s %-10s %-18s %s\n", id, "project", "missing", model, "")
	}
	return nil
}

func runSession(args []string) error {
	if len(args) == 0 {
		return errors.New("session requires list, status, show, resume, or export")
	}
	switch args[0] {
	case "list":
		return runSessionList(args[1:])
	case "status":
		return runSessionStatus(args[1:])
	case "show":
		return runSessionShow(args[1:])
	case "recovery":
		return runSessionRecovery(args[1:])
	case "export":
		return runSessionExport(args[1:])
	case "resume":
		return runSessionResume(args)
	default:
		return fmt.Errorf("unknown session subcommand %q (use: list, status, show, resume, export)", args[0])
	}
}

func runSessionList(args []string) error {
	flags := flag.NewFlagSet("session list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	result, err := runner.SessionList(ctx)
	if err != nil {
		return fmt.Errorf("session list: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(result.Sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}
	fmt.Printf("%-24s %-10s %-8s %s\n", "BRANCH ID", "STATUS", "SCOPE", "TURN")
	for _, s := range result.Sessions {
		fmt.Printf("%-24s %-10s %-8s %d\n", s.BranchID, s.Status, s.Scope, s.Turn)
	}
	return nil
}

func runSessionStatus(args []string) error {
	flags := flag.NewFlagSet("session status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("session status requires a branch-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	detail, err := runner.SessionStatus(ctx, flags.Arg(0))
	if err != nil {
		return fmt.Errorf("session status: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(detail)
	}
	fmt.Printf("Branch ID:  %s\n", detail.BranchID)
	fmt.Printf("Status:     %s\n", detail.Status)
	if detail.Scope != "" {
		fmt.Printf("Scope:      %s\n", detail.Scope)
	}
	fmt.Printf("Turn:       %d\n", detail.Turn)
	fmt.Printf("Lifecycle:  %s\n", detail.Lifecycle)
	fmt.Printf("Recovered:  %t\n", detail.Recovered)
	fmt.Printf("Schema:     %d\n", detail.SchemaVersion)
	return nil
}

func runSessionShow(args []string) error {
	flags := flag.NewFlagSet("session show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("session show requires a branch-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	detail, err := runner.SessionShow(ctx, flags.Arg(0))
	if err != nil {
		return fmt.Errorf("session show: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(detail)
	}
	fmt.Printf("Branch ID:  %s\n", detail.BranchID)
	fmt.Printf("Status:     %s\n", detail.Status)
	fmt.Printf("Turn:       %d\n", detail.Turn)
	fmt.Printf("Lifecycle:  %s\n", detail.Lifecycle)
	fmt.Printf("Schema:     %d\n", detail.SchemaVersion)
	return nil
}

func runSessionRecovery(args []string) error {
	flags := flag.NewFlagSet("session recovery", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	branchID := ""
	if flags.NArg() > 0 {
		branchID = flags.Arg(0)
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	info, err := runner.SessionRecovery(ctx, branchID)
	if err != nil {
		return fmt.Errorf("session recovery: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	fmt.Printf("Branch ID:    %s\n", info.BranchID)
	fmt.Printf("Status:       %s\n", info.Status)
	fmt.Printf("Tasks Total:  %d\n", info.TasksTotal)
	fmt.Printf("Tasks Failed: %d\n", info.TasksFailed)
	return nil
}

func runSessionResume(args []string) error {
	flags := flag.NewFlagSet("session resume", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	copySession := flags.Bool("copy", false, "resume a duplicated session")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	path := *binary
	if !filepath.IsAbs(path) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("Reasonix executable not found: %w", err)
		}
		path = resolved
	}
	commandArgs := []string{"--continue"}
	if *copySession {
		commandArgs = append(commandArgs, "--copy")
	}
	cmd := exec.Command(path, commandArgs...)
	cmd.Dir = *projectDir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runSessionExport(args []string) error {
	flags := flag.NewFlagSet("session export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	out := flags.String("out", "", "diagnostic zip output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		return errors.New("session export requires a branch id or session path")
	}
	path := *binary
	if !filepath.IsAbs(path) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("Reasonix executable not found: %w", err)
		}
		path = resolved
	}
	commandArgs := []string{"doctor", "session", flags.Arg(0), "--zip"}
	if *out != "" {
		commandArgs = append(commandArgs, "--out", *out)
	}
	cmd := exec.Command(path, commandArgs...)
	cmd.Dir = *projectDir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runBenchmark(args []string) error {
	if len(args) == 0 {
		return errors.New("benchmark requires cache or quality")
	}
	switch args[0] {
	case "cache":
		return runCacheBenchmark(args[1:])
	case "quality":
		return runQualityBenchmark(args[1:])
	default:
		return fmt.Errorf("unknown benchmark %q", args[0])
	}
}

func runCacheBenchmark(args []string) error {
	flags := flag.NewFlagSet("benchmark cache", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	trace := flags.String("trace", "", "JSONL request trace")
	nativeTrace := flags.String("native-trace", "", "Native JSONL request trace for comparison")
	omrTrace := flags.String("omr-trace", "", "OMR JSONL request trace for comparison")
	output := flags.String("output", "", "optional JSON report path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *nativeTrace != "" || *omrTrace != "" {
		if *nativeTrace == "" || *omrTrace == "" {
			return errors.New("benchmark cache comparison requires both --native-trace and --omr-trace")
		}
		native, err := cacheguard.ReadJSONL(*nativeTrace)
		if err != nil {
			return err
		}
		omr, err := cacheguard.ReadJSONL(*omrTrace)
		if err != nil {
			return err
		}
		comparison := cacheguard.CompareReports(native, omr)
		if err := writeJSONReport(*output, comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return errors.New("cache comparison failed hard gates")
		}
		return nil
	}
	if *trace == "" {
		return errors.New("benchmark cache requires --trace")
	}
	report, err := cacheguard.ReadJSONL(*trace)
	if err != nil {
		return err
	}
	if *output == "" {
		return cacheguard.WriteReport(os.Stdout, report)
	}
	file, err := os.Create(*output)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := cacheguard.WriteReport(file, report); err != nil {
		return err
	}
	fmt.Printf("cache report: %s\n", *output)
	if !report.Passed {
		return errors.New("cache benchmark failed hard gates")
	}
	return nil
}

func writeJSONReport(path string, value interface{}) error {
	writer := os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("cache report: %s\n", path)
	}
	return nil
}

func runQualityBenchmark(args []string) error {
	flags := flag.NewFlagSet("benchmark quality", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fixturesRoot := flags.String("fixtures", "benchmarks/fixtures", "fixture root")
	resultsPath := flags.String("results", "", "optional JSON map of fixture id to RunResult")
	nativeResultsPath := flags.String("native-results", "", "Native JSON results for quality comparison")
	omrResultsPath := flags.String("omr-results", "", "OMR JSON results for quality comparison")
	outputPath := flags.String("output", "", "optional JSON report path")
	replay := flags.Bool("replay", false, "run fixtures with deterministic replay outcomes")
	paired := flags.Bool("paired", false, "run native/omr paired replay comparison (requires full-flow fixtures with native_replay/omr_replay)")
	runtimeRun := flags.Bool("runtime", false, "run fixtures through the real Reasonix CLI")
	runTests := flags.Bool("run-tests", false, "run fixture hidden and regression tests")
	projectDir := flags.String("project-dir", ".", "project directory for fixture tests")
	binary := flags.String("binary", "reasonix", "Reasonix executable for --runtime")
	metricsDir := flags.String("metrics-dir", "", "metrics output directory for --runtime")
	eventsPath := flags.String("events", "", "optional JSONL structured event log for --runtime")
	model := flags.String("model", "", "optional Reasonix model for --runtime")
	maxSteps := flags.Int("max-steps", 0, "optional Reasonix step limit for --runtime")
	concurrency := flags.Int("concurrency", 1, "maximum concurrent --runtime fixtures")
	timeout := flags.Duration("timeout", 2*time.Minute, "per benchmark execution timeout")
	minQualifiedRate := flags.Float64("min-qualified-rate", 1, "fail when qualified rate is below this value (0..1)")
	maxCost := flags.Float64("max-cost", 0, "optional aggregate cost budget; 0 disables the gate")
	configPath := flags.String("config", "", "optional OMR config (TOML or JSONC; default: <project>/.reasonix/omr/config.jsonc or config.toml)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runID := "omr-" + time.Now().Format("20060102-150405")
	configFile := *configPath
	if configFile == "" {
		configFile = omrconfig.FindConfig(*projectDir)
	}
	if cfg, configErr := omrconfig.Load(configFile); configErr == nil {
		if !flagWasSet(flags, "fixtures") && cfg.Fixtures != "" {
			*fixturesRoot = projectRelativePath(*projectDir, cfg.Fixtures)
		}
		if !flagWasSet(flags, "metrics-dir") && cfg.MetricsDir != "" {
			*metricsDir = projectRelativePath(*projectDir, cfg.MetricsDir)
		}
		if !flagWasSet(flags, "model") && cfg.Model != "" {
			*model = cfg.Model
		}
		if !flagWasSet(flags, "max-steps") && cfg.MaxSteps != 0 {
			*maxSteps = cfg.MaxSteps
		}
		if !flagWasSet(flags, "concurrency") && cfg.Concurrency != 0 {
			*concurrency = cfg.Concurrency
		}
		if !flagWasSet(flags, "timeout") && cfg.TimeoutSet {
			*timeout = cfg.Timeout
		}
		if !flagWasSet(flags, "min-qualified-rate") && cfg.MinQualifiedRateSet {
			*minQualifiedRate = cfg.MinQualifiedRate
		}
		if !flagWasSet(flags, "max-cost") && cfg.MaxCostSet {
			*maxCost = cfg.MaxCost
		}
	} else if !os.IsNotExist(configErr) {
		return fmt.Errorf("load OMR config: %w", configErr)
	}
	fixtures, err := qualitybench.Discover(*fixturesRoot)
	if err != nil {
		return err
	}
	if *runtimeRun && (*replay || *resultsPath != "") {
		return errors.New("--runtime cannot be combined with --replay or --results")
	}
	if *nativeResultsPath != "" || *omrResultsPath != "" {
		if *nativeResultsPath == "" || *omrResultsPath == "" || *replay || *runtimeRun || *resultsPath != "" {
			return errors.New("quality comparison requires only --native-results and --omr-results")
		}
		native, err := loadQualityResults(*nativeResultsPath)
		if err != nil {
			return err
		}
		omr, err := loadQualityResults(*omrResultsPath)
		if err != nil {
			return err
		}
		comparison := qualitybench.CompareReports(
			qualitybench.EvaluateAll(fixtures, native, runID, qualitybench.ExecutionModeReplay),
			qualitybench.EvaluateAll(fixtures, omr, runID, qualitybench.ExecutionModeReplay),
		)
		if err := writeJSONValue(*outputPath, comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return errors.New("quality comparison failed hard gate")
		}
		if err := qualitybench.CheckCostGate(comparison.OMR, *maxCost); err != nil {
			return fmt.Errorf("quality comparison cost gate failed: %w", err)
		}
		return nil
	}
	if *runtimeRun {
		if *concurrency < 1 {
			return errors.New("--concurrency must be at least 1")
		}
		if *eventsPath != "" && *concurrency > 1 {
			return errors.New("--events requires --concurrency 1 because one event stream cannot be safely shared")
		}
		results := map[string]qualitybench.RunResult{}
		var mu sync.Mutex
		sem := make(chan struct{}, *concurrency)
		var wg sync.WaitGroup
		for _, fixture := range fixtures {
			fixture := fixture
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				result, runErr := qualitybench.ExecuteRuntime(ctx, fixture, *projectDir, *binary, *metricsDir, *model, *maxSteps)
				cancel()
				if runErr == nil && *eventsPath != "" {
					if events, eventErr := qualitybench.ReadEventNames(*eventsPath); eventErr == nil {
						result.Events = events
					}
				}
				if runErr != nil {
					result.Failed = true
					if result.Error == "" {
						result.Error = runErr.Error()
					}
				}
				mu.Lock()
				results[fixture.ID] = result
				mu.Unlock()
			}()
		}
		wg.Wait()
		report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeRuntime)
		if err := writeJSONValue(*outputPath, report); err != nil {
			return err
		}
		if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
			return fmt.Errorf("quality runtime failed: %w", err)
		}
		return nil
	}
	if *paired {
		nativeResults := map[string]qualitybench.RunResult{}
		omrResults := map[string]qualitybench.RunResult{}
		for _, fixture := range fixtures {
			native, omr, err := qualitybench.ReplayPaired(fixture)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: paired replay skipped %q: %v\n", fixture.ID, err)
				continue
			}
			nativeResults[fixture.ID] = native
			omrResults[fixture.ID] = omr
		}
		nativeReport := qualitybench.EvaluateAll(fixtures, nativeResults, runID, qualitybench.ExecutionModePaired)
		omrReport := qualitybench.EvaluateAll(fixtures, omrResults, runID, qualitybench.ExecutionModePaired)
		if nativeReport.EvaluatedCount == 0 {
			return errors.New("no fixtures contain native_replay data; use --paired only on fixtures with native_replay/omr_replay")
		}
		comparison := qualitybench.CompareReports(nativeReport, omrReport)
		if err := writeJSONValue(*outputPath, comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return fmt.Errorf("paired comparison failed: native=%d/%d omr=%d/%d",
				nativeReport.QualifiedCount, nativeReport.EvaluatedCount,
				omrReport.QualifiedCount, omrReport.EvaluatedCount)
		}
		return nil
	}
	if *replay {
		results := map[string]qualitybench.RunResult{}
		for _, fixture := range fixtures {
			var result qualitybench.RunResult
			var replayErr error
			if *runTests {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				result, replayErr = qualitybench.ExecuteFixture(ctx, fixture, *projectDir)
				cancel()
			} else {
				result, replayErr = qualitybench.Replay(fixture)
			}
			if replayErr != nil {
				results[fixture.ID] = qualitybench.RunResult{
					Failed: true,
					Error:  replayErr.Error(),
				}
				continue
			}
			results[fixture.ID] = result
		}
		report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeReplay)
		if err := writeJSONValue(*outputPath, report); err != nil {
			return err
		}
		if report.EvaluatedCount == 0 {
			return errors.New("no fixtures contain replay outcomes")
		}
		if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
			return fmt.Errorf("quality replay failed: %w", err)
		}
		return nil
	}
	if *resultsPath == "" {
		fmt.Printf("quality fixtures: %d\n", len(fixtures))
		for _, fixture := range fixtures {
			fmt.Printf("- %s: %s\n", fixture.ID, fixture.Task)
		}
		fmt.Println("no --results supplied; execution is intentionally separate from scoring")
		return nil
	}
	results, err := loadQualityResults(*resultsPath)
	if err != nil {
		return err
	}
	report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeReplay)
	if err := writeJSONValue(*outputPath, report); err != nil {
		return err
	}
	if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
		return fmt.Errorf("quality benchmark failed: %w", err)
	}
	return nil
}

func checkQualityGates(report qualitybench.Report, minimumRate, maximumCost float64) error {
	if err := qualitybench.CheckGate(report, minimumRate); err != nil {
		return err
	}
	return qualitybench.CheckCostGate(report, maximumCost)
}

func projectRelativePath(projectDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectDir, path)
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func loadQualityResults(path string) (map[string]qualitybench.RunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	results := map[string]qualitybench.RunResult{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse quality results: %w", err)
	}
	return results, nil
}

func writeJSONValue(path string, value interface{}) error {
	writer := os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("quality report: %s\n", path)
	}
	return nil
}

func loadAssetsFromInvocation() (install.Assets, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return install.Assets{}, err
	}
	return install.LoadAssets(cwd)
}

func runVersion(args []string) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "output version info as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *jsonOutput {
		type versionInfo struct {
			OMR        string `json:"omr_version"`
			Manifest   string `json:"manifest_schema"`
			Assets     string `json:"assets_version"`
			Reasonix   string `json:"reasonix_detected"`
			Compatible bool   `json:"compatible"`
		}
		info := versionInfo{
			OMR:        version,
			Manifest:   "1",
			Assets:     "builtin",
			Reasonix:   "",
			Compatible: true,
		}
		// Try to detect Reasonix binary
		if path, lookErr := exec.LookPath("reasonix"); lookErr == nil {
			if data, execErr := exec.Command(path, "version").Output(); execErr == nil {
				info.Reasonix = strings.TrimSpace(string(data))
			} else {
				info.Reasonix = "detected but version check failed"
			}
		} else {
			info.Reasonix = "not found in PATH"
			info.Compatible = false
		}
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	fmt.Printf("omr %s\n", version)
	// Check Reasonix presence
	if path, lookErr := exec.LookPath("reasonix"); lookErr == nil {
		fmt.Printf("reasonix: %s\n", path)
	} else {
		fmt.Println("reasonix: not found in PATH")
	}
	return nil
}

func runHook(args []string) error {
	// Strip "doctor" subcommand name so flag parsing sees the flags.
	if len(args) > 0 && args[0] == "doctor" {
		args = args[1:]
	}
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

	// Call both hook list and hook status
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
		return json.NewEncoder(os.Stdout).Encode(hookDoctorOutput{List: listResult, Status: statusResult})
	}
	if len(listResult.Hooks) == 0 {
		fmt.Println("No hooks found")
	}
	fmt.Printf("%-20s %-10s %-8s %s\n", "HOOK", "STATUS", "EVENT", "SCOPE")
	for _, h := range listResult.Hooks {
		fmt.Printf("%-20s %-10s %-8s %s\n", h.Name, h.Status, h.Event, h.Scope)
	}
	if statusResult.Unavailable {
		fmt.Printf("STATUS: unavailable — %s\n", statusResult.Error)
	} else {
		fmt.Printf("STATUS: active=%d inactive=%d untrusted=%d\n",
			len(statusResult.Active), len(statusResult.Inactive), len(statusResult.Untrusted))
	}
	return nil
}

func runTask(args []string) error {
	if len(args) == 0 || (args[0] != "list" && args[0] != "show") {
		return errors.New("task requires list or show")
	}
	if args[0] == "show" {
		return runTaskShow(args[1:])
	}
	return runTaskList(args[1:])
}

func runTaskList(args []string) error {
	flags := flag.NewFlagSet("task list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	result, err := runner.TaskList(ctx, *sessionID)
	if err != nil {
		return fmt.Errorf("task list: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(result.Tasks) == 0 {
		fmt.Println("No tasks found")
		return nil
	}
	fmt.Printf("%-24s %-10s %-8s %s\n", "TASK ID", "STATUS", "TYPE", "STEP")
	for _, t := range result.Tasks {
		fmt.Printf("%-24s %-10s %-8s %d\n", t.ID, t.Status, t.Type, t.Step)
	}
	return nil
}

func runTaskShow(args []string) error {
	flags := flag.NewFlagSet("task show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	sessionID := flags.String("session", "", "session ID for the task")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("task show requires a task-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()
	detail, err := runner.TaskShow(ctx, flags.Arg(0), *sessionID)
	if err != nil {
		return fmt.Errorf("task show: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(detail)
	}
	fmt.Printf("Task ID:    %s\n", detail.ID)
	fmt.Printf("Status:     %s\n", detail.Status)
	fmt.Printf("Type:       %s\n", detail.Type)
	fmt.Printf("Step:       %d\n", detail.Step)
	fmt.Printf("Session:    %s\n", detail.SessionID)
	return nil
}

func runRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	eventsJSONL := flags.String("events-jsonl", "", "path to write structured events JSONL")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("run requires a task prompt")
	}
	prompt := flags.Arg(0)

	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()

	if *eventsJSONL != "" {
		result := runner.RunWithEvents(ctx, prompt, *eventsJSONL)
		// Always parse events (file is saved even on non-zero exit).
		stream, parseErr := reasonix.ParseEventStream(*eventsJSONL)
		if parseErr != nil {
			return fmt.Errorf("parse events: %w", parseErr)
		}
		if len(stream.Errors) > 0 {
			return fmt.Errorf("event stream validation failed: %s", strings.Join(stream.Errors, "; "))
		}
		if result.Err != nil {
			return fmt.Errorf("run failed (exit %d): %w", result.ExitCode, result.Err)
		}
		if *jsonOutput {
			type runOutput struct {
				Result reasonix.Result      `json:"result"`
				Events reasonix.EventStream `json:"events"`
			}
			return json.NewEncoder(os.Stdout).Encode(runOutput{Result: result, Events: stream})
		}
		fmt.Printf("Run completed (exit %d)\n", result.ExitCode)
		fmt.Printf("Events: %d, run_done=%t\n", len(stream.Events), stream.RunDone)
		if len(stream.Errors) > 0 {
			for _, e := range stream.Errors {
				fmt.Printf("  event error: %s\n", e)
			}
		}
		return nil
	}
	result := runner.RunTask(ctx, reasonix.TaskOptions{Prompt: prompt})
	if result.Err != nil {
		return fmt.Errorf("run task: %w", result.Err)
	}
	fmt.Print(result.Stdout)
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	return nil
}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Printf("%s init|upgrade|uninstall|doctor|config|profile|session|benchmark|version\n", name)
	fmt.Println("Use --help on a command for flags.")
}
