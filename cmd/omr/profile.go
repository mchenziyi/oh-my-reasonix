package main

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
