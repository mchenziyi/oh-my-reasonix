package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

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
