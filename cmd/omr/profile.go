package main

import (
	"errors"
	"os"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
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
