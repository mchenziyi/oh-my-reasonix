package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/commenthook"
	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/promptcompose"
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
	omrConfigPath := omrconfig.FindConfig(root)
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
	var hookList reasonix.HookListOutput
	var hookErr error
	var reasonixHooksAvailable bool
	binary, err := resolveReasonixBinary()
	if err != nil {
		result.Warnings = append(result.Warnings, "reasonix executable not found in PATH; runtime capability checks skipped")
	} else {
		result.Checks = append(result.Checks, Check{Name: "reasonix.binary", Status: "PASS", Detail: "found: " + binary})
		runner := reasonix.Runner{Binary: binary, ProjectDir: root}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		probe, probeErr := runner.Probe(ctx)
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
		hookCtx, hookCancel := context.WithTimeout(context.Background(), 5*time.Second)
		hookList, hookErr = runner.HookList(hookCtx)
		hookStatus := runner.HookStatus(hookCtx)
		hookCancel()
		hookCheck := Check{Name: "reasonix.hooks"}
		if hookErr == nil && !hookStatus.Unavailable {
			hookCheck.Status = "PASS"
			hookCheck.Detail = fmt.Sprintf("hook list/status available (%d hook(s), %d source(s))",
				len(hookList.Hooks), len(hookStatus.Sources))
		} else {
			hookCheck.Status = "UNSUPPORTED"
			hookCheck.Detail = "Reasonix Hook 查询接口不可用"
			if hookErr != nil {
				hookCheck.Detail += ": " + hookErr.Error()
			} else if hookStatus.Error != "" {
				hookCheck.Detail += ": " + hookStatus.Error
			}
		}
		result.Checks = append(result.Checks, hookCheck)
		reasonixHooksAvailable = hookErr == nil && !hookStatus.Unavailable
	}
	manifestPath := install.ManifestPathForDoctor(root)
	if err := commenthook.ValidateManagedPath(manifestPath, root); err != nil {
		result.Errors = append(result.Errors, "unsafe OMR manifest path: "+err.Error())
		return result, err
	}
	m, err := manifest.Load(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Errors = append(result.Errors, "OMR manifest not found; run omr init")
			return result, fmt.Errorf("manifest not found")
		}
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}
	result.Checks = append(result.Checks, Check{Name: "manifest", Status: "PASS", Detail: "schema and required fields valid"})
	if m.Evolution != nil && m.Evolution.OverlaySHA256 != "" {
		p := filepath.Join(root, m.Evolution.OverlayPath)
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			result.Errors = append(result.Errors, "evolution overlay missing: "+readErr.Error())
		} else if got := promptcompose.SHA256String(string(b)); got != m.Evolution.OverlaySHA256 {
			result.Errors = append(result.Errors, "evolution overlay hash drift")
		} else {
			result.Checks = append(result.Checks, Check{Name: "evolution.overlay", Status: "PASS", Detail: got})
		}
	}
	commentHookCheck := commentHookDiagnostic(root, reasonixHooksAvailable, hookList, m.Hook)
	result.Checks = append(result.Checks, commentHookCheck)
	if commentHookCheck.Status == "ERROR" {
		result.Errors = append(result.Errors, "comment-hook: "+commentHookCheck.Detail)
	}
	if hasOMRConfig && (len(omrConfig.Agents) > 0 || len(omrConfig.Categories) > 0 || len(omrConfig.DisabledProfiles) > 0) {
		installed := make(map[string]bool)
		for _, profile := range m.NormalizedProfiles() {
			installed[profile.ID] = true
		}
		agentProfiles := make([]string, 0, len(omrConfig.Agents))
		for profile := range omrConfig.Agents {
			agentProfiles = append(agentProfiles, profile)
		}
		sort.Strings(agentProfiles)
		for _, profile := range agentProfiles {
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR config references uninstalled Profile %q", profile))
			}
		}
		disabledProfiles := append([]string(nil), omrConfig.DisabledProfiles...)
		sort.Strings(disabledProfiles)
		for _, profile := range disabledProfiles {
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR config disables uninstalled Profile %q", profile))
			}
		}
		categories := make([]string, 0, len(omrConfig.Categories))
		for category := range omrConfig.Categories {
			categories = append(categories, category)
		}
		sort.Strings(categories)
		for _, category := range categories {
			profile := omrConfig.Categories[category]
			if !installed[profile] {
				result.Errors = append(result.Errors, fmt.Sprintf("OMR category %q references uninstalled Profile %q", category, profile))
			}
		}
		for _, category := range omrConfig.DisabledRoutingConflicts() {
			result.Errors = append(result.Errors, fmt.Sprintf("OMR category %q routes to disabled Profile %q", category, omrConfig.Categories[category]))
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
	if hasOMRConfig {
		for _, diagnostic := range omrconfig.DiagnoseMCP(omrConfig) {
			status := "PASS"
			switch diagnostic.Availability {
			case "disabled":
				status = "DISABLED"
			case "unavailable":
				status = "WARN"
				result.Warnings = append(result.Warnings, fmt.Sprintf("MCP server %q: %s", diagnostic.Server, diagnostic.Summary()))
			}
			if diagnostic.Enabled && diagnostic.Compatibility != "compatible" {
				status = "WARN"
				result.Warnings = append(result.Warnings, fmt.Sprintf("MCP server %q has unknown capabilities; %s", diagnostic.Server, diagnostic.Summary()))
			}
			result.Checks = append(result.Checks, Check{
				Name:   "omr.config.mcp." + diagnostic.Server,
				Status: status,
				Detail: diagnostic.Summary(),
			})
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
		// Profile metadata check
		skillPath := install.ProfilePath(root, profile.Path)
		if data, readErr := os.ReadFile(skillPath); readErr == nil {
			meta, parseErr := manifest.ParseProfileMeta(data)
			if parseErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("profile.%s metadata: %v", profile.ID, parseErr))
			} else {
				if meta.Description == "" {
					result.Warnings = append(result.Warnings, fmt.Sprintf("profile.%s metadata: missing description", profile.ID))
				}
				if len(meta.AllowedTools) == 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("profile.%s metadata: no allowed-tools declared", profile.ID))
				}
			}
		}
	}
	for _, drift := range install.PromptSourceDrift(root, m, assets) {
		result.Errors = append(result.Errors, sourceDriftMessage(drift))
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

func resolveReasonixBinary() (string, error) {
	configured := strings.TrimSpace(os.Getenv("OMR_REASONIX_BIN"))
	if configured == "" {
		return exec.LookPath("reasonix")
	}
	if filepath.IsAbs(configured) {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return configured, nil
		}
		return "", fmt.Errorf("OMR_REASONIX_BIN is not an executable file: %s", configured)
	}
	return exec.LookPath(configured)
}

func sourceDriftMessage(drift string) string {
	switch drift {
	case "Reasonix base Prompt source hash changed":
		return drift + "; rerun `omr upgrade --accept-reasonix-base-update`"
	case "OMR Orchestrator Prompt source hash changed":
		return drift + "; rerun `omr upgrade`"
	case "User Prompt source hash changed", "User Prompt source is missing":
		return drift + "; inspect the configured user Prompt source before upgrading"
	default:
		return drift
	}
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

// commentHookDiagnostic checks the Comment Checker Hook status.
// It is diagnostic-only (no writes).
func commentHookDiagnostic(root string, reasonixHooksAvailable bool, hookList reasonix.HookListOutput, hookRecord *manifest.HookRecord) Check {
	omrPath, resolveErr := commenthook.ResolveOmrPath()
	return commentHookDiagnosticWithExecutable(root, reasonixHooksAvailable, hookList, omrPath, resolveErr, hookRecord)
}

func commentHookDiagnosticWithExecutable(root string, reasonixHooksAvailable bool, hookList reasonix.HookListOutput, omrPath string, resolveErr error, hookRecord *manifest.HookRecord) Check {
	settingsPath := commenthook.SettingsPath(root)
	if err := commenthook.ValidateManagedPath(settingsPath, root); err != nil {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "settings 路径不安全: " + err.Error()}
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if hookRecord != nil && hookRecord.Enabled {
				return Check{Name: "comment-hook", Status: "ERROR", Detail: "Manifest 声明 Hook 已启用，但 settings.json 不存在"}
			}
			return Check{Name: "comment-hook", Status: "WARN", Detail: "未启用（默认状态）；settings.json 不存在"}
		}
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "读取 settings.json 失败: " + err.Error()}
	}

	parsed, parseErr := commenthook.ParseSettings(raw)
	if parseErr != nil {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "settings.json 解析失败: " + parseErr.Error()}
	}

	entries := parsed.Hooks["PreToolUse"]
	enabled := false
	drifted := false
	legacy := false
	markerCount := 0
	var ownedRaw []byte
	for _, re := range entries {
		if re.HasOMRDescription() {
			markerCount++
			if re.IsOMROwnedFor(omrPath) {
				enabled = true
				ownedRaw = re.Raw
				if entry, ok := re.Entry(); ok && entry.Command == commenthook.OMRCommandLegacy {
					legacy = true
				}
			} else {
				drifted = true
			}
		}
	}

	if markerCount > 1 {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "检测到多个 OMR Hook 条目；需要人工处理"}
	}

	if !enabled && !drifted {
		if hookRecord != nil && hookRecord.Enabled {
			return Check{Name: "comment-hook", Status: "ERROR", Detail: "Manifest 声明 Hook 已启用，但 settings 中没有 OMR 条目"}
		}
		return Check{Name: "comment-hook", Status: "WARN", Detail: "未启用（默认状态）；使用 `omr hook comment-check enable` 启用"}
	}

	if markerCount > 0 && resolveErr != nil {
		// The omr executable cannot be resolved (moved, removed, or PATH
		// changed). Report the root cause before claiming the entry drifted:
		// an absolute-path entry cannot be verified against a missing binary.
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "无法解析稳定的 omr 可执行路径；已安装 Hook 无法验证且 command 无法执行"}
	}

	if drifted {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "OMR Hook 条目已被修改，与规范不一致"}
	}

	if hookRecord == nil {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "settings 中存在 OMR Hook，但 Manifest 缺少 Hook 所有权记录"}
	}
	if !hookRecord.Enabled {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "settings 中存在 OMR Hook，但 Manifest 将其标记为禁用"}
	}
	if hookRecord.SettingsPath != commenthook.HookSettingsRel ||
		hookRecord.Event != "PreToolUse" ||
		hookRecord.Description != commenthook.OMRDescription {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "Manifest Hook 所有权字段与规范不一致"}
	}
	if hookRecord.EntrySHA256 != fileutil.SHA256(ownedRaw) {
		return Check{Name: "comment-hook", Status: "ERROR", Detail: "Manifest Hook 条目哈希与 settings 不一致"}
	}
	if hookRecord.InstalledFileSHA256 != fileutil.SHA256(raw) {
		return Check{Name: "comment-hook", Status: "WARN", Detail: "OMR Hook 条目有效，但 settings 文件包含安装后的其它变更；可重跑 enable 刷新 Manifest 证据"}
	}

	if legacy {
		return Check{Name: "comment-hook", Status: "WARN", Detail: "已启用旧版 PATH 依赖命令；请重新运行 `omr hook comment-check enable` 迁移为绝对路径"}
	}

	if !reasonixHooksAvailable {
		return Check{Name: "comment-hook", Status: "UNSUPPORTED", Detail: "Reasonix 不支持 Hook 或 Hook 查询接口不可用"}
	}

	// Check if Reasonix can see the hook.
	visible := false
	for _, h := range hookList.Hooks {
		if h.Event == "PreToolUse" && h.Match == "bash" && h.Scope == "project" && (h.Status == "active" || h.Status == "enabled") {
			visible = true
			break
		}
	}
	if !visible {
		return Check{Name: "comment-hook", Status: "WARN", Detail: "已启用但 Reasonix hook list 中不可见（可能需重启 Reasonix）"}
	}

	return Check{Name: "comment-hook", Status: "PASS", Detail: "已启用，条目规范，omr 可执行路径有效，Reasonix 可见"}
}
