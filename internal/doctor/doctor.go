package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

type Result struct {
	Root     string   `json:"root"`
	Checks   []Check  `json:"checks"`
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
}

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (r Result) Blocking() bool { return len(r.Errors) > 0 }

func (r Result) Render(w ioWriter) {
	fmt.Fprintf(w, "project: %s\n", r.Root)
	for _, check := range r.Checks {
		fmt.Fprintf(w, "%s %s: %s\n", check.Status, check.Name, check.Detail)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "WARNING: %s\n", warning)
	}
	for _, err := range r.Errors {
		fmt.Fprintf(w, "ERROR: %s\n", err)
	}
}

type ioWriter interface {
	Write([]byte) (int, error)
}

func Run(projectDir string, assets install.Assets) (Result, error) {
	root, err := install.ProjectRoot(projectDir)
	if err != nil {
		return Result{}, err
	}
	result := Result{Root: root, Checks: []Check{}, Warnings: []string{}, Errors: []string{}}
	var omrConfig omrconfig.Config
	var hasOMRConfig bool
	configPath := filepath.Join(root, "reasonix.toml")
	if _, err := os.Stat(configPath); err != nil {
		result.Errors = append(result.Errors, "reasonix.toml not found")
		return result, fmt.Errorf("reasonix.toml not found")
	}
	result.Checks = append(result.Checks, Check{Name: "reasonix.config", Status: "PASS", Detail: configPath})
	omrConfigPath := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if _, statErr := os.Stat(omrConfigPath); statErr == nil {
		if loaded, configErr := omrconfig.Load(omrConfigPath); configErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid OMR config: %v", configErr))
		} else {
			omrConfig = loaded
			hasOMRConfig = true
			result.Checks = append(result.Checks, Check{Name: "omr.config", Status: "PASS", Detail: omrConfigPath})
		}
	} else if !os.IsNotExist(statErr) {
		result.Errors = append(result.Errors, fmt.Sprintf("read OMR config: %v", statErr))
	}
	binary, err := exec.LookPath("reasonix")
	if err != nil {
		result.Warnings = append(result.Warnings, "reasonix executable not found in PATH; runtime capability checks skipped")
	} else {
		result.Checks = append(result.Checks, Check{Name: "reasonix.binary", Status: "PASS", Detail: "found in PATH"})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		probe, probeErr := (reasonix.Runner{Binary: binary, ProjectDir: root}).Probe(ctx)
		cancel()
		if probeErr != nil {
			result.Errors = append(result.Errors, probeErr.Error())
		} else {
			for _, capability := range probe.Checks {
				if capability.Available {
					result.Checks = append(result.Checks, Check{Name: "reasonix." + capability.Name, Status: "PASS", Detail: capability.Detail})
					continue
				}
				if capability.Name == "version" {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Reasonix capability %q unavailable: %s", capability.Name, capability.Detail))
					continue
				}
				result.Errors = append(result.Errors, fmt.Sprintf("Reasonix capability %q unavailable: %s", capability.Name, capability.Detail))
			}
		}
	}
	m, err := manifest.Load(install.ManifestPathForDoctor(root))
	if err != nil {
		if os.IsNotExist(err) {
			result.Errors = append(result.Errors, "OMR manifest not found; run omr init")
			return result, fmt.Errorf("manifest not found")
		}
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}
	result.Checks = append(result.Checks, Check{Name: "manifest", Status: "PASS", Detail: "schema and required fields valid"})
	if hasOMRConfig && (len(omrConfig.Agents) > 0 || len(omrConfig.Categories) > 0 || len(omrConfig.DisabledProfiles) > 0) {
		installed := make(map[string]bool)
		for _, profile := range m.NormalizedProfiles() {
			installed[profile.ID] = true
		}
		for profile := range omrConfig.Agents {
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR config references uninstalled Profile %q", profile))
			}
		}
		for _, profile := range omrConfig.DisabledProfiles {
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR config disables uninstalled Profile %q", profile))
			}
		}
		for category, profile := range omrConfig.Categories {
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR category %q references uninstalled Profile %q", category, profile))
			}
			for _, disabled := range omrConfig.DisabledProfiles {
				if profile == disabled {
					result.Errors = append(result.Errors, fmt.Sprintf("OMR category %q routes to disabled Profile %q", category, profile))
				}
			}
		}
		for profile, agent := range omrConfig.Agents {
			if agent.PromptFile == "" {
				continue
			}
			promptPath := agent.PromptFile
			if !filepath.IsAbs(promptPath) {
				promptPath = filepath.Join(root, promptPath)
			}
			info, statErr := os.Stat(promptPath)
			if statErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Prompt file for Profile %q is unavailable: %v", profile, statErr))
			} else if info.IsDir() {
				result.Errors = append(result.Errors, fmt.Sprintf("Prompt file for Profile %q is a directory: %s", profile, agent.PromptFile))
			}
			if agent.ReadOnly != nil && *agent.ReadOnly {
				for _, installedProfile := range m.NormalizedProfiles() {
					if installedProfile.ID != profile {
						continue
					}
					profileData, readErr := os.ReadFile(install.ProfilePath(root, installedProfile.Path))
					if readErr != nil {
						break
					}
					if !profileFrontmatterReadOnly(string(profileData)) {
						result.Errors = append(result.Errors, fmt.Sprintf("Profile %q is configured read_only but its Skill is not read-only", profile))
					}
				}
			}
		}
		if len(result.Errors) == 0 {
			result.Checks = append(result.Checks, Check{Name: "omr.config.profiles", Status: "PASS", Detail: "all configured Profiles and categories are installed"})
		}
		if len(omrConfig.Categories) > 0 && len(result.Errors) == 0 {
			result.Checks = append(result.Checks, Check{Name: "omr.config.routing", Status: "PASS", Detail: fmt.Sprintf("%d category routes configured", len(omrConfig.Categories))})
		}
		if omrConfig.Concurrency > 0 && len(result.Errors) == 0 {
			result.Checks = append(result.Checks, Check{Name: "omr.config.concurrency", Status: "PASS", Detail: fmt.Sprintf("runtime concurrency=%d", omrConfig.Concurrency)})
		}
		if omrConfig.MaxCost > 0 && len(result.Errors) == 0 {
			result.Checks = append(result.Checks, Check{Name: "omr.config.max_cost", Status: "PASS", Detail: fmt.Sprintf("quality cost budget=%.4f", omrConfig.MaxCost)})
		}
		if len(omrConfig.DisabledProfiles) > 0 && len(result.Errors) == 0 {
			result.Checks = append(result.Checks, Check{Name: "omr.config.disabled", Status: "PASS", Detail: fmt.Sprintf("%d Profiles disabled", len(omrConfig.DisabledProfiles))})
		}
	}
	generated := install.GeneratedPromptPathForDoctor(root)
	if actual, err := fileutil.SHA256File(generated); err != nil || actual != m.Prompt.FinalSHA256 {
		result.Errors = append(result.Errors, "generated Prompt hash drift detected")
	} else {
		result.Checks = append(result.Checks, Check{Name: "prompt.hash", Status: "PASS", Detail: m.Prompt.FinalSHA256})
	}
	for _, profile := range m.NormalizedProfiles() {
		path := install.ProfilePath(root, profile.Path)
		if actual, err := fileutil.SHA256File(path); err != nil || actual != profile.ContentSHA256 {
			result.Errors = append(result.Errors, profile.ID+" Profile hash drift detected")
		} else {
			result.Checks = append(result.Checks, Check{Name: "profile." + profile.ID, Status: "PASS", Detail: profile.Path})
		}
	}
	for _, drift := range install.PromptSourceDrift(root, m, assets) {
		result.Errors = append(result.Errors, drift)
	}
	if len(result.Errors) == 0 {
		result.Checks = append(result.Checks, Check{Name: "prompt.sources", Status: "PASS", Detail: "source hashes match Manifest"})
	}
	for _, name := range []string{"review", "security-review", "security_review"} {
		path := filepath.Join(root, ".reasonix", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			result.Errors = append(result.Errors, fmt.Sprintf("built-in review Profile %q is shadowed by project file %s", name, path))
		}
	}
	if len(result.Errors) == 0 {
		result.Checks = append(result.Checks, Check{Name: "review.integration", Status: "PASS", Detail: "no project Profile shadows built-in review"})
	}
	if assets.Root == "" {
		result.Warnings = append(result.Warnings, "asset source is not available; source drift check skipped")
	} else {
		result.Checks = append(result.Checks, Check{Name: "asset.source", Status: "PASS", Detail: assets.Root})
	}
	return result, resultError(result)
}

func profileFrontmatterReadOnly(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "read-only: true" {
			return true
		}
	}
	return false
}

func resultError(result Result) error {
	if len(result.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("doctor found %d blocking issue(s)", len(result.Errors))
}
