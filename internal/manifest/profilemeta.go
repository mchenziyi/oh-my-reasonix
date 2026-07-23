package manifest

import (
	"fmt"
	"sort"
	"strings"
)

// ProfileMeta holds structured metadata extracted from a SKILL.md file.
type ProfileMeta struct {
	ID              string   `json:"id"`
	Description     string   `json:"description,omitempty"`
	ReadOnly        bool     `json:"read_only"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	InputTypes      []string `json:"input_types,omitempty"`
	OutputSections  []string `json:"output_sections,omitempty"`
	FailureGuidance string   `json:"failure_guidance,omitempty"`
}

// ParseProfileMeta parses a SKILL.md file into structured metadata.
// Uses simple line-by-line frontmatter parsing (no YAML dependency).
func ParseProfileMeta(content []byte) (ProfileMeta, error) {
	meta := ProfileMeta{FailureGuidance: "返回发现事实给父任务"}
	text := string(content)

	// Find frontmatter
	if !strings.HasPrefix(text, "---\n") {
		return meta, fmt.Errorf("missing frontmatter delimiter ---")
	}
	endIdx := strings.Index(text[len("---\n"):], "\n---")
	if endIdx < 0 {
		return meta, fmt.Errorf("frontmatter delimiter --- not closed")
	}
	frontMatter := text[len("---\n") : len("---\n")+endIdx]

	// Parse frontmatter fields
	fields := map[string]string{}
	for _, line := range strings.Split(frontMatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		fields[key] = val
	}

	meta.ID = fields["name"]
	meta.Description = fields["description"]
	if strings.EqualFold(fields["read-only"], "true") {
		meta.ReadOnly = true
	}

	// Parse allowed-tools from frontmatter (e.g., "[read_file, grep, glob]")
	if tools, ok := fields["allowed-tools"]; ok {
		tools = strings.Trim(tools, "[]")
		for _, t := range strings.Split(tools, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				meta.AllowedTools = append(meta.AllowedTools, t)
			}
		}
	}
	sort.Strings(meta.AllowedTools)

	// Parse body sections (after frontmatter closing ---)
	body := text[len("---\n")+endIdx+len("\n---"):]
	lines := strings.Split(body, "\n")
	currentSection := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## 输入") {
			currentSection = "input"
			continue
		}
		if strings.HasPrefix(trimmed, "## 输出") {
			currentSection = "output"
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			currentSection = ""
			continue
		}
		if currentSection == "input" && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimPrefix(trimmed, "- ")
			if idx := strings.Index(item, "："); idx > 0 {
				item = item[:idx]
			} else if idx := strings.Index(item, ":"); idx > 0 {
				item = item[:idx]
			}
			meta.InputTypes = append(meta.InputTypes, item)
		}
		if currentSection == "output" && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "1. ") || strings.HasPrefix(trimmed, "2. ")) {
			item := strings.TrimLeft(trimmed, "- 1234567890. ")
			meta.OutputSections = append(meta.OutputSections, item)
		}
	}
	return meta, nil
}
