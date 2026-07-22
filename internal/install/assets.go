package install

import (
	"fmt"
	"os"
	"path/filepath"

	embeddedassets "github.com/mchenziyi/oh-my-reasonix/assets"
)

type Assets struct {
	Root         string
	BasePrompt   []byte
	Orchestrator []byte
	Explore      []byte
	Research     []byte
	Debug        []byte
	Planner      []byte
	ReviewBrief  []byte
}

func LoadAssets(start string) (Assets, error) {
	root, err := findAssetRoot(start)
	if err == nil {
		return readAssetsFromDirectory(root)
	}
	if os.Getenv("OMR_ASSET_DIR") != "" {
		return Assets{}, err
	}
	return Assets{
		Root:         "embedded",
		BasePrompt:   append([]byte(nil), embeddedassets.BasePrompt...),
		Orchestrator: append([]byte(nil), embeddedassets.Orchestrator...),
		Explore:      append([]byte(nil), embeddedassets.Explore...),
		Research:     append([]byte(nil), embeddedassets.Research...),
		Debug:        append([]byte(nil), embeddedassets.Debug...),
		Planner:      append([]byte(nil), embeddedassets.Planner...),
		ReviewBrief:  append([]byte(nil), embeddedassets.ReviewBrief...),
	}, nil
}

func readAssetsFromDirectory(root string) (Assets, error) {
	read := func(rel string) ([]byte, error) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", path, err)
		}
		return data, nil
	}
	base, err := read("prompts/reasonix-base-464d494.md")
	if err != nil {
		return Assets{}, err
	}
	orchestrator, err := read("prompts/orchestrator.zh.md")
	if err != nil {
		return Assets{}, err
	}
	explore, err := read("skills/omr-explore/SKILL.md")
	if err != nil {
		return Assets{}, err
	}
	research, err := read("skills/omr-research/SKILL.md")
	if err != nil {
		return Assets{}, err
	}
	debug, err := read("skills/omr-debug/SKILL.md")
	if err != nil {
		return Assets{}, err
	}
	planner, err := read("skills/omr-planner/SKILL.md")
	if err != nil {
		return Assets{}, err
	}
	review, err := read("prompts/review-task-protocol.zh.md")
	if err != nil {
		return Assets{}, err
	}
	return Assets{Root: root, BasePrompt: base, Orchestrator: orchestrator, Explore: explore, Research: research, Debug: debug, Planner: planner, ReviewBrief: review}, nil
}

func findAssetRoot(start string) (string, error) {
	if configured := os.Getenv("OMR_ASSET_DIR"); configured != "" {
		if dirExists(configured) {
			return filepath.Abs(configured)
		}
		return "", fmt.Errorf("OMR_ASSET_DIR does not exist: %s", configured)
	}
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
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(abs, "assets")
		if fileExists(filepath.Join(candidate, "prompts", "reasonix-base-464d494.md")) {
			return candidate, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return "", fmt.Errorf("OMR assets not found; run from the repository or set OMR_ASSET_DIR")
}
