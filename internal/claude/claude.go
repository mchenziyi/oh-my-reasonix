package claude

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Claude 项目目录结构常量
const (
	RulesDir     = ".claude/rules"
	SkillsDir    = ".claude/skills"
	AgentsDir    = ".claude/agents"
	CommandsDir  = ".claude/commands"
	MCPFile      = ".claude/mcp.json"
	HooksDir     = ".claude/hooks"
	OMRRulesDir  = ".reasonix/rules"
	OMRSkillsDir = ".reasonix/skills"
)

// Options 控制 Claude 导入行为。
type Options struct {
	ProjectDir string
	DryRun     bool
	Force      bool // 覆盖已有冲突
}

// Report 描述一次导入操作的结果。
type Report struct {
	Root      string
	Changes   []Change
	Warnings  []string
	Conflicts []string
	Errors    []string
	NoOp      bool
	Written   bool
	Status    string // "planned" | "written" | "skipped" | "conflict" | "error"
}

// inferStatus 根据报告字段自动推断 Status。
func (r *Report) inferStatus() {
	if len(r.Errors) > 0 {
		r.Status = "error"
	} else if len(r.Conflicts) > 0 && !r.Written {
		r.Status = "conflict"
	} else if r.Written {
		r.Status = "written"
	} else if r.NoOp {
		r.Status = "skipped"
	} else if len(r.Changes) > 0 {
		r.Status = "planned"
	} else {
		r.Status = "skipped"
	}
}

// Change 描述单个文件的操作。
type Change struct {
	Path   string
	Action string // IMPORT, SKIP, CONFLICT
	Detail string
}

// Render 输出人类可读的报告。
func (r Report) Render(w io.Writer) {
	r.inferStatus()
	if len(r.Errors) > 0 {
		for _, e := range r.Errors {
			fmt.Fprintf(w, "ERROR: %s\n", e)
		}
		return
	}
	if len(r.Conflicts) > 0 && !r.Written {
		fmt.Fprintf(w, "CONFLICT:\n")
		for _, c := range r.Conflicts {
			fmt.Fprintf(w, "  %s\n", c)
		}
		fmt.Fprintf(w, "Use --force to overwrite.\n")
		return
	}
	if r.NoOp {
		fmt.Fprintf(w, "NOOP: nothing to import\n")
		return
	}
	if r.Written {
		fmt.Fprintf(w, "IMPORTED:\n")
		for _, c := range r.Changes {
			fmt.Fprintf(w, "  %s  %s\n", c.Action, c.Path)
			if c.Detail != "" {
				fmt.Fprintf(w, "    %s\n", c.Detail)
			}
		}
		for _, warn := range r.Warnings {
			fmt.Fprintf(w, "WARNING: %s\n", warn)
		}
		return
	}
	// Dry-run
	fmt.Fprintf(w, "PLAN:\n")
	for _, c := range r.Changes {
		fmt.Fprintf(w, "  %s  %s\n", c.Action, c.Path)
		if c.Detail != "" {
			fmt.Fprintf(w, "    %s\n", c.Detail)
		}
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "WARNING: %s\n", warn)
	}
}

// RenderJSON 输出机器可读的 JSON 报告。
func (r Report) RenderJSON(w io.Writer) {
	r.inferStatus()
	type jsonReport struct {
		Status    string   `json:"status"`
		Root      string   `json:"root"`
		Changes   []Change `json:"changes,omitempty"`
		Warnings  []string `json:"warnings,omitempty"`
		Conflicts []string `json:"conflicts,omitempty"`
		Errors    []string `json:"errors,omitempty"`
		NoOp      bool     `json:"noop"`
		Written   bool     `json:"written"`
	}
	jr := jsonReport{
		Status:    r.Status,
		Root:      r.Root,
		Changes:   r.Changes,
		Warnings:  r.Warnings,
		Conflicts: r.Conflicts,
		Errors:    r.Errors,
		NoOp:      r.NoOp,
		Written:   r.Written,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(jr)
}

// ProjectRoot 从项目目录向上查找 .git 或 reasonix.toml。
func ProjectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for p := abs; p != filepath.Dir(p); p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p, nil
		}
		if _, err := os.Stat(filepath.Join(p, "reasonix.toml")); err == nil {
			return p, nil
		}
	}
	return abs, nil
}

// RuleFile represents a parsed Claude rule.
type RuleFile struct {
	SourceRel string // relative to .claude/rules/
	Content   []byte
}

// DiscoverRules finds all .md files in .claude/rules/.
func DiscoverRules(root string) ([]RuleFile, error) {
	rulesDir := filepath.Join(root, filepath.FromSlash(RulesDir))
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rules []RuleFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(rulesDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read rule %q: %w", entry.Name(), err)
		}
		rules = append(rules, RuleFile{
			SourceRel: entry.Name(),
			Content:   data,
		})
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].SourceRel < rules[j].SourceRel
	})
	return rules, nil
}

// contentHash returns the SHA256 hex of data.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// fileHash returns the SHA256 hex of a file's content, or "" if not found.
func fileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return contentHash(data)
}
