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

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

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

func runProfile(args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("profile requires list")
	}
	return runProfileList(args[1:])
}

func loadProfileConfig(root string) (map[string]omrconfig.AgentConfig, map[string][]string, map[string]bool, error) {
	configured := map[string]omrconfig.AgentConfig{}
	categoryByProfile := map[string][]string{}
	disabled := map[string]bool{}
	configPath := omrconfig.FindConfig(root)
	if _, statErr := os.Stat(configPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return configured, categoryByProfile, disabled, nil
		}
		return nil, nil, nil, statErr
	}
	cfg, err := omrconfig.Load(configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	configured = cfg.Agents
	for category, profile := range cfg.Categories {
		categoryByProfile[profile] = append(categoryByProfile[profile], category)
	}
	for _, profile := range cfg.DisabledProfiles {
		disabled[profile] = true
	}
	return configured, categoryByProfile, disabled, nil
}

func applyProfileMetadata(item *profileJSON, root, profilePath string) {
	skillPath := install.ProfilePath(root, profilePath)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return
	}
	meta, err := manifest.ParseProfileMeta(data)
	if err != nil {
		return
	}
	item.Description = meta.Description
	item.ReadOnlyBool = meta.ReadOnly
	item.AllowedTools = meta.AllowedTools
	item.InputTypes = meta.InputTypes
	item.OutputSections = meta.OutputSections
}

func applyProfileAgentConfig(item *profileJSON, root string, agent omrconfig.AgentConfig) {
	item.Model, item.PromptFile, item.ReadOnly = agent.Model, agent.PromptFile, agent.ReadOnly
	if agent.Model != "" {
		item.EffectiveModel = agent.Model
		item.ModelSource = "project"
	}
	if agent.PromptFile == "" {
		return
	}
	promptPath := agent.PromptFile
	if !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(root, promptPath)
	}
	exists := false
	if info, err := os.Stat(promptPath); err == nil && !info.IsDir() {
		exists = true
	}
	item.PromptFileExists = &exists
}

func applyProfileRouting(item *profileJSON, categories map[string][]string, disabled map[string]bool) {
	item.Categories = append([]string(nil), categories[item.ID]...)
	sort.Strings(item.Categories)
	item.Disabled = disabled[item.ID]
	if item.Disabled {
		item.Status = "disabled"
	}
}

func projectOnlyProfileIDs(profiles []manifest.Profile, configured map[string]omrconfig.AgentConfig) []string {
	installed := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		installed[profile.ID] = true
	}
	ids := make([]string, 0)
	for id := range configured {
		if !installed[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
