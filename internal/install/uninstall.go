package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func Uninstall(opts Options) (Report, error) {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{}, err
	}
	m, err := manifest.Load(ManifestPath(root))
	if err != nil {
		return Report{Root: root, Errors: []string{fmt.Sprintf("load manifest: %v", err)}}, err
	}
	configPath, err := requireReasonixConfig(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}
	cfg := parseAgentConfig(string(configData))
	report := Report{Root: root, Manifest: m}

	newConfig := string(configData)
	configChanged := false
	for _, entry := range m.Config {
		if entry.Path != "agent.system_prompt_file" {
			continue
		}
		current := valueOrEmpty(cfg.SystemPromptFile)
		installed := entry.InstalledValue
		base := ""
		basePresent := entry.BaseValue != nil
		if basePresent {
			base = *entry.BaseValue
		}
		switch {
		case current == installed:
			var err error
			newConfig, err = setOrRemoveAgentFile(newConfig, base, basePresent)
			if err != nil {
				return reportWithError(report, err)
			}
			configChanged = newConfig != string(configData)
		case current == base && !basePresent:
			// The user already removed the key.
		case current == base:
			// The user already restored the original value.
		default:
			report.Conflicts = append(report.Conflicts, "agent.system_prompt_file was modified after OMR installation; current value preserved")
		}
	}

	generatedPath := GeneratedPromptPath(root)
	if fileExists(generatedPath) {
		if fileHashDiffers(generatedPath, m.Prompt.FinalSHA256) {
			report.Conflicts = append(report.Conflicts, "generated Prompt was modified; file preserved")
		} else {
			report.Changes = append(report.Changes, Change{Path: GeneratedPromptRel, Action: "REMOVE", Detail: "restore OMR-generated Prompt"})
		}
	}
	profilePath := ExploreProfilePath(root)
	if fileExists(profilePath) {
		if fileHashDiffers(profilePath, m.ProfileSHA256) {
			report.Conflicts = append(report.Conflicts, "omr-explore Profile was modified; file preserved")
		} else {
			report.Changes = append(report.Changes, Change{Path: ExploreProfileRel, Action: "REMOVE", Detail: "remove OMR-owned Profile"})
		}
	}
	if configChanged {
		report.Changes = append(report.Changes, Change{Path: "reasonix.toml", Action: "UPDATE", Detail: "field-level restore of agent.system_prompt_file"})
	}
	if len(report.Conflicts) > 0 {
		return report, fmt.Errorf("uninstall blocked by conflicts")
	}
	if opts.DryRun {
		return report, nil
	}

	oldConfig := configData
	oldGenerated, generatedExisted := readIfExists(generatedPath)
	oldProfile, profileExisted := readIfExists(profilePath)
	oldManifest, manifestExisted := readIfExists(ManifestPath(root))
	rollback := func() {
		if configChanged {
			restoreFile(configPath, true, oldConfig)
		}
		if generatedExisted {
			_ = fileutil.AtomicWrite(generatedPath, oldGenerated, 0o644)
		}
		if profileExisted {
			_ = fileutil.AtomicWrite(profilePath, oldProfile, 0o644)
		}
		if manifestExisted {
			_ = fileutil.AtomicWrite(ManifestPath(root), oldManifest, 0o644)
		}
	}
	if configChanged {
		if err := fileutil.AtomicWrite(configPath, []byte(newConfig), 0o644); err != nil {
			return reportWithError(report, err)
		}
	}
	if fileExists(generatedPath) {
		if err := os.Remove(generatedPath); err != nil {
			rollback()
			return reportWithError(report, err)
		}
	}
	if fileExists(profilePath) {
		if err := os.Remove(profilePath); err != nil {
			rollback()
			return reportWithError(report, err)
		}
	}
	if err := os.Remove(ManifestPath(root)); err != nil && !os.IsNotExist(err) {
		rollback()
		return reportWithError(report, err)
	}
	if m.BackupPath != "" {
		_ = os.RemoveAll(filepath.Join(root, filepath.FromSlash(m.BackupPath)))
	}
	removeEmptyDir(filepath.Dir(generatedPath))
	removeEmptyDir(filepath.Dir(profilePath))
	removeEmptyDir(filepath.Join(root, ".reasonix", "omr", "backups"))
	removeEmptyDir(filepath.Join(root, ".reasonix", "omr"))
	removeEmptyDir(filepath.Join(root, ".reasonix", "skills"))
	removeEmptyDir(filepath.Join(root, ".reasonix"))
	report.Written = true
	report.Result = "uninstalled"
	return report, nil
}

func removeEmptyDir(path string) {
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		return
	}
	_ = os.Remove(path)
}

func setOrRemoveAgentFile(text, value string, present bool) (string, error) {
	if present {
		return replaceOrAppendAgentFile(text, value)
	}
	cfg := parseAgentConfig(text)
	if !cfg.SystemPromptFile.Present {
		return text, nil
	}
	lines := append([]string(nil), cfg.Lines...)
	lines = append(lines[:cfg.SystemPromptFile.Line], lines[cfg.SystemPromptFile.Line+1:]...)
	result := strings.Join(lines, "\n")
	if cfg.HadTrailingLF && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

func reportWithError(report Report, err error) (Report, error) {
	report.Errors = append(report.Errors, err.Error())
	return report, err
}
