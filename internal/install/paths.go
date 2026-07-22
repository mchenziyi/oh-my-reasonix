package install

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	GeneratedPromptRel = ".reasonix/omr/generated/system-prompt.md"
	ManifestRel        = ".reasonix/omr/manifest.lock.yaml"
	ExploreProfileRel  = ".reasonix/skills/omr-explore/SKILL.md"
	ResearchProfileRel = ".reasonix/skills/omr-research/SKILL.md"
	DebugProfileRel    = ".reasonix/skills/omr-debug/SKILL.md"
	PlannerProfileRel  = ".reasonix/skills/omr-planner/SKILL.md"
	FrontendProfileRel = ".reasonix/skills/omr-frontend/SKILL.md"
)

func ProjectRoot(start string) (string, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	fallback := abs
	for {
		if fileExists(filepath.Join(abs, "reasonix.toml")) || dirExists(filepath.Join(abs, ".git")) {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	// A new project may not have a config or repository yet. The explicit
	// project directory remains the least surprising installation target.
	return fallback, nil
}

func ManifestPath(root string) string { return filepath.Join(root, filepath.FromSlash(ManifestRel)) }
func GeneratedPromptPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(GeneratedPromptRel))
}
func ExploreProfilePath(root string) string {
	return filepath.Join(root, filepath.FromSlash(ExploreProfileRel))
}
func ProfilePath(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// Exported path helpers keep diagnostics and benchmark packages independent
// of the installer implementation details.
func ManifestPathForDoctor(root string) string        { return ManifestPath(root) }
func GeneratedPromptPathForDoctor(root string) string { return GeneratedPromptPath(root) }
func ExploreProfilePathForDoctor(root string) string  { return ExploreProfilePath(root) }
func ExploreProfileRelForDoctor() string              { return ExploreProfileRel }

func requireReasonixConfig(root string) (string, error) {
	path := filepath.Join(root, "reasonix.toml")
	if !fileExists(path) {
		return "", fmt.Errorf("reasonix.toml not found in project root %s", root)
	}
	return path, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func relOrSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func samePath(root, configured, target string) bool {
	if configured == "" {
		return false
	}
	clean := filepath.Clean(configured)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(root, clean)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absConfigured, err := filepath.Abs(clean)
	if err != nil {
		return false
	}
	return filepath.Clean(absConfigured) == filepath.Clean(absTarget)
}
