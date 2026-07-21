package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

type Result struct {
	Root     string
	Checks   []Check
	Warnings []string
	Errors   []string
}

type Check struct {
	Name   string
	Status string
	Detail string
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
	result := Result{Root: root}
	configPath := filepath.Join(root, "reasonix.toml")
	if _, err := os.Stat(configPath); err != nil {
		result.Errors = append(result.Errors, "reasonix.toml not found")
		return result, fmt.Errorf("reasonix.toml not found")
	}
	result.Checks = append(result.Checks, Check{Name: "reasonix.config", Status: "PASS", Detail: configPath})
	if _, err := exec.LookPath("reasonix"); err != nil {
		result.Warnings = append(result.Warnings, "reasonix executable not found in PATH; runtime capability checks skipped")
	} else {
		result.Checks = append(result.Checks, Check{Name: "reasonix.binary", Status: "PASS", Detail: "found in PATH"})
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
	generated := install.GeneratedPromptPathForDoctor(root)
	if actual, err := fileutil.SHA256File(generated); err != nil || actual != m.Prompt.FinalSHA256 {
		result.Errors = append(result.Errors, "generated Prompt hash drift detected")
	} else {
		result.Checks = append(result.Checks, Check{Name: "prompt.hash", Status: "PASS", Detail: m.Prompt.FinalSHA256})
	}
	profile := install.ExploreProfilePathForDoctor(root)
	if actual, err := fileutil.SHA256File(profile); err != nil || actual != m.ProfileSHA256 {
		result.Errors = append(result.Errors, "omr-explore Profile hash drift detected")
	} else {
		result.Checks = append(result.Checks, Check{Name: "profile.omr-explore", Status: "PASS", Detail: install.ExploreProfileRelForDoctor()})
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

func resultError(result Result) error {
	if len(result.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("doctor found %d blocking issue(s)", len(result.Errors))
}
