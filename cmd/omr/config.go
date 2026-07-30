package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
)

type configValidationMissing struct {
	Path       string `json:"path"`
	Valid      bool   `json:"valid"`
	Configured bool   `json:"configured"`
}

type configValidationError struct {
	Path       string   `json:"path"`
	Valid      bool     `json:"valid"`
	Configured bool     `json:"configured"`
	Error      string   `json:"error"`
	Errors     []string `json:"errors"`
}

type configValidationOutput struct {
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

func knownOMRProfiles() map[string]bool {
	return map[string]bool{
		"omr-explore": true, "omr-research": true, "omr-debug": true,
		"omr-planner": true, "omr-frontend": true, "omr-git": true,
		"omr-lsp": true, "omr-grill-me": true, "omr-grill-with-docs": true,
	}
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
