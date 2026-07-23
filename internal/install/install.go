package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/promptcompose"
)

type Options struct {
	ProjectDir               string
	DryRun                   bool
	ComposePrompt            bool
	AllowPersistUserPrompt   bool
	AcceptReasonixBaseUpdate bool
	Upgrade                  bool
	Assets                   Assets
}

type Change struct {
	Path   string
	Action string
	Detail string
}

type Report struct {
	Root      string
	Changes   []Change
	Warnings  []string
	Conflicts []string
	Errors    []string
	NoOp      bool
	Written   bool
	Result    string
	Manifest  manifest.Manifest
}

func (r Report) Blocking() bool { return len(r.Conflicts) > 0 || len(r.Errors) > 0 }

type profileAsset struct {
	ID   string
	Rel  string
	Data []byte
	Hash string
}

func (r Report) Render(w io.Writer) {
	fmt.Fprintf(w, "project: %s\n", r.Root)
	for _, change := range r.Changes {
		fmt.Fprintf(w, "PLAN %s %s: %s\n", change.Action, change.Path, change.Detail)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "WARNING: %s\n", warning)
	}
	for _, conflict := range r.Conflicts {
		fmt.Fprintf(w, "CONFLICT: %s\n", conflict)
	}
	for _, err := range r.Errors {
		fmt.Fprintf(w, "ERROR: %s\n", err)
	}
	if r.NoOp {
		fmt.Fprintln(w, "NOOP: already installed and unchanged")
	}
	if r.Written {
		if r.Result == "" {
			r.Result = "completed"
		}
		fmt.Fprintf(w, "RESULT: %s\n", r.Result)
	}
}

func Init(opts Options) (Report, error) {
	root, err := ProjectRoot(opts.ProjectDir)
	if err != nil {
		return Report{}, err
	}
	if opts.Assets.Root == "" {
		opts.Assets, err = LoadAssets(root)
		if err != nil {
			return Report{Root: root, Errors: []string{err.Error()}}, err
		}
	}
	configPath, err := requireReasonixConfig(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}
	if opts.Upgrade && !fileExists(ManifestPath(root)) {
		return Report{Root: root, Errors: []string{"omr upgrade requires an existing OMR manifest"}}, fmt.Errorf("manifest not found")
	}

	oldConfig, err := os.ReadFile(configPath)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}
	cfg := parseAgentConfig(string(oldConfig))
	existing, hasManifest, err := loadManifest(root)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}

	generatedPath := GeneratedPromptPath(root)
	manifestOwnedPrompt := hasManifest && existing.Prompt.GeneratedPath == GeneratedPromptRel && samePath(root, valueOrEmpty(cfg.SystemPromptFile), generatedPath)

	// Check for session event-index files that may conflict with OMR management
	if orphans := getOrphanEventPaths(root); len(orphans) > 0 {
		return conflictReport(root, fmt.Sprintf("existing session event-index file(s) found (may conflict with OMR): %s", strings.Join(orphans, ", ")))
	}

	if samePath(root, valueOrEmpty(cfg.SystemPromptFile), generatedPath) && !manifestOwnedPrompt {
		return conflictReport(root, "system_prompt_file points to the OMR generated path but the manifest is missing or does not claim it")
	}

	userSource, userPrompt, userPresent, err := resolveUserPrompt(root, cfg, existing, hasManifest, manifestOwnedPrompt, opts.ComposePrompt)
	if err != nil {
		return conflictReport(root, err.Error())
	}
	if (cfg.SystemPromptFile.Present || cfg.SystemPrompt.Present) && !manifestOwnedPrompt && !opts.ComposePrompt {
		return conflictReport(root, "existing agent.system_prompt_file/system_prompt requires --compose-prompt")
	}

	baseText := string(opts.Assets.BasePrompt)
	orchestratorText := string(opts.Assets.Orchestrator)
	omrConfigPath := omrconfig.FindConfig(root)
	if omrCfg, configErr := omrconfig.Load(omrConfigPath); configErr == nil {
		orchestratorText += omrCfg.CategoryPrompt() + omrCfg.DisabledProfilePrompt() + omrCfg.MCPPrompt()
	} else if !os.IsNotExist(configErr) {
		return Report{Root: root, Errors: []string{fmt.Sprintf("load OMR config: %v", configErr)}}, configErr
	}
	composition := promptcompose.Compose(baseText, userPrompt, orchestratorText)
	profiles := profileAssets(opts.Assets)

	report := Report{Root: root, Manifest: existing}
	if hasManifest && existing.Prompt.BaseSHA256 != promptcompose.SHA256String(promptcompose.Canonicalize(baseText)) && !opts.AcceptReasonixBaseUpdate {
		report.Conflicts = append(report.Conflicts, "Reasonix base Prompt changed; rerun with --accept-reasonix-base-update")
	}

	if conflict := checkAssetPathConflict(root, generatedPath, profiles, existing, hasManifest); conflict != "" {
		report.Conflicts = append(report.Conflicts, conflict)
	}

	newConfig, err := replaceOrAppendAgentFile(string(oldConfig), GeneratedPromptRel)
	if err != nil {
		return Report{Root: root, Errors: []string{err.Error()}}, err
	}
	configChanged := newConfig != string(oldConfig)
	profilesChanged := profilesNeedWrite(root, profiles)
	generatedChanged := !fileExists(generatedPath) || fileHashDiffers(generatedPath, composition.Hash)

	if userPresent {
		report.Warnings = append(report.Warnings, "User Prompt content will be persisted in the generated Prompt and backup paths")
		if !opts.AllowPersistUserPrompt && (configChanged || generatedChanged || profilesChanged || !hasManifest) {
			if opts.DryRun {
				report.Warnings = append(report.Warnings, "installation is blocked without --allow-persist-user-prompt")
			} else {
				report.Conflicts = append(report.Conflicts, "non-empty User Prompt requires --allow-persist-user-prompt")
			}
		}
	}

	if report.Blocking() {
		return report, fmt.Errorf("installation blocked by conflicts")
	}

	backupRel := existing.BackupPath
	if backupRel == "" {
		backupRel = filepath.ToSlash(filepath.Join(".reasonix/omr/backups", composition.Hash[:12]))
	}
	baseValue := stringPointerIfPresent(cfg.SystemPromptFile)
	if manifestOwnedPrompt && hasManifest && len(existing.Config) > 0 {
		// The installed value is not the original base value. Preserve the
		// three-way merge baseline recorded by the first installation.
		baseValue = existing.Config[0].BaseValue
	}
	orchestratorSourceHash := promptcompose.SHA256String(promptcompose.Canonicalize(string(opts.Assets.Orchestrator)))
	newManifest := buildManifest(composition, orchestratorSourceHash, profiles, userSource, userPresent, baseValue, backupRel)
	manifestChanged := !hasManifest || !manifestsEqual(existing, newManifest)
	if !configChanged && !generatedChanged && !profilesChanged && !manifestChanged {
		report.NoOp = true
		report.Manifest = existing
		return report, nil
	}

	report.Changes = appendInstallChanges(report.Changes, root, configChanged, generatedChanged, profilesChanged, profiles, manifestChanged, backupRel)
	report.Manifest = newManifest
	if opts.DryRun {
		return report, nil
	}
	if err := writeInstall(root, configPath, oldConfig, newConfig, generatedPath, []byte(composition.Content), profiles, ManifestPath(root), newManifest, backupRel, configChanged, generatedChanged, profilesChanged, manifestChanged); err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	report.Written = true
	report.Result = "installed"
	return report, nil
}

func resolveUserPrompt(root string, cfg agentConfig, existing manifest.Manifest, hasManifest, manifestOwned bool, compose bool) (source, value string, present bool, err error) {
	if manifestOwned {
		if cfg.SystemPrompt.Present {
			return "inline", cfg.SystemPrompt.Value, promptcompose.Canonicalize(cfg.SystemPrompt.Value) != "", nil
		}
		if hasManifest && existing.Prompt.UserPresent && existing.Prompt.UserSource != "" && existing.Prompt.UserSource != "inline" {
			value, err := readPromptSource(root, existing.Prompt.UserSource)
			if err != nil {
				return "", "", false, err
			}
			return existing.Prompt.UserSource, value, promptcompose.Canonicalize(value) != "", nil
		}
		return "", "", false, nil
	}
	if !compose {
		return "", "", false, nil
	}
	if cfg.SystemPromptFile.Present {
		value, err := readPromptSource(root, cfg.SystemPromptFile.Value)
		if err != nil {
			return "", "", false, err
		}
		return cfg.SystemPromptFile.Value, value, promptcompose.Canonicalize(value) != "", nil
	}
	if cfg.SystemPrompt.Present {
		return "inline", cfg.SystemPrompt.Value, promptcompose.Canonicalize(cfg.SystemPrompt.Value) != "", nil
	}
	return "", "", false, nil
}

func profileAssets(assets Assets) []profileAsset {
	profiles := []profileAsset{{
		ID:   "omr-explore",
		Rel:  ExploreProfileRel,
		Data: assets.Explore,
		Hash: fileutil.SHA256(assets.Explore),
	}}
	if len(assets.Research) > 0 {
		profiles = append(profiles, profileAsset{
			ID:   "omr-research",
			Rel:  ResearchProfileRel,
			Data: assets.Research,
			Hash: fileutil.SHA256(assets.Research),
		})
	}
	if len(assets.Debug) > 0 {
		profiles = append(profiles, profileAsset{
			ID:   "omr-debug",
			Rel:  DebugProfileRel,
			Data: assets.Debug,
			Hash: fileutil.SHA256(assets.Debug),
		})
	}
	if len(assets.Planner) > 0 {
		profiles = append(profiles, profileAsset{ID: "omr-planner", Rel: PlannerProfileRel, Data: assets.Planner, Hash: fileutil.SHA256(assets.Planner)})
	}
	if len(assets.Frontend) > 0 {
		profiles = append(profiles, profileAsset{ID: "omr-frontend", Rel: FrontendProfileRel, Data: assets.Frontend, Hash: fileutil.SHA256(assets.Frontend)})
	}
	if len(assets.Git) > 0 {
		profiles = append(profiles, profileAsset{ID: "omr-git", Rel: GitProfileRel, Data: assets.Git, Hash: fileutil.SHA256(assets.Git)})
	}
	if len(assets.LSP) > 0 {
		profiles = append(profiles, profileAsset{ID: "omr-lsp", Rel: LSPProfileRel, Data: assets.LSP, Hash: fileutil.SHA256(assets.LSP)})
	}
	return profiles
}

func profilesNeedWrite(root string, profiles []profileAsset) bool {
	for _, profile := range profiles {
		path := ProfilePath(root, profile.Rel)
		if !fileExists(path) || fileHashDiffers(path, profile.Hash) {
			return true
		}
	}
	return false
}

func checkAssetPathConflict(root, generatedPath string, profiles []profileAsset, existing manifest.Manifest, hasManifest bool) string {
	if fileExists(generatedPath) {
		owned := hasManifest && existing.Prompt.GeneratedPath == GeneratedPromptRel
		if !owned {
			return "generated Prompt file exists but is not claimed by the OMR manifest"
		}
		if fileHashDiffers(generatedPath, existing.Prompt.FinalSHA256) {
			return "generated Prompt file was modified after installation"
		}
	}
	ownedProfiles := map[string]string{}
	if hasManifest {
		for _, profile := range existing.NormalizedProfiles() {
			ownedProfiles[profile.Path] = profile.ContentSHA256
		}
	}
	for _, profile := range profiles {
		path := ProfilePath(root, profile.Rel)
		if fileExists(path) {
			hash, owned := ownedProfiles[profile.Rel]
			if !owned {
				return profile.ID + " Profile already exists and is not OMR-owned"
			}
			if fileHashDiffers(path, hash) {
				return "OMR-owned " + profile.ID + " Profile was modified after installation"
			}
		}
	}
	return ""
}

func buildManifest(composition promptcompose.Composition, orchestratorSourceHash string, profiles []profileAsset, userSource string, userPresent bool, baseValue *string, backupRel string) manifest.Manifest {
	m := manifest.New()
	m.Prompt = manifest.Prompt{
		GeneratedPath:      GeneratedPromptRel,
		BaseSource:         "assets/prompts/reasonix-base-464d494.md",
		BaseSHA256:         composition.Segments[0].Hash,
		UserPresent:        userPresent,
		UserSource:         userSource,
		OrchestratorSource: "assets/prompts/orchestrator.zh.md",
		OrchestratorSHA256: orchestratorSourceHash,
		FinalSHA256:        composition.Hash,
	}
	if userPresent {
		for _, segment := range composition.Segments {
			if segment.ID == "user" {
				m.Prompt.UserSHA256 = segment.Hash
				break
			}
		}
	}
	m.Assets = []manifest.Asset{
		{ID: "reasonix-base-464d494", Role: "system_prompt_segment", SourceProject: "reasonix", SourceVersion: "desktop-v1.17.16", SourceCommit: "464d494", SourcePath: "assets/prompts/reasonix-base-464d494.md", LicenseStatus: "upstream-public-source", ContentSHA256: composition.Segments[0].Hash, InstallTarget: GeneratedPromptRel, CompositionOrder: 1},
		{ID: "orchestrator.zh", Role: "system_prompt_segment", SourceProject: "clean-room", SourceVersion: manifest.Version, SourcePath: "assets/prompts/orchestrator.zh.md", LicenseStatus: "project-owned", ContentSHA256: orchestratorSourceHash, InstallTarget: GeneratedPromptRel, CompositionOrder: len(composition.Segments)},
	}
	for _, profile := range profiles {
		m.Profiles = append(m.Profiles, manifest.Profile{ID: profile.ID, Path: profile.Rel, ContentSHA256: profile.Hash})
		m.Assets = append(m.Assets, manifest.Asset{ID: profile.ID, Role: "profile", SourceProject: "clean-room", SourceVersion: manifest.Version, SourcePath: "assets/skills/" + profile.ID + "/SKILL.md", LicenseStatus: "project-owned", ContentSHA256: profile.Hash, InstallTarget: profile.Rel})
	}
	m.Config = []manifest.ConfigEntry{{Path: "agent.system_prompt_file", BaseValue: baseValue, InstalledValue: GeneratedPromptRel}}
	if len(profiles) > 0 {
		m.ProfilePath = profiles[0].Rel
		m.ProfileSHA256 = profiles[0].Hash
	}
	m.BackupPath = backupRel
	return m
}

func appendInstallChanges(changes []Change, root string, configChanged, generatedChanged, profilesChanged bool, profiles []profileAsset, manifestChanged bool, backupRel string) []Change {
	if configChanged {
		changes = append(changes, Change{Path: relOrSlash(root, filepath.Join(root, "reasonix.toml")), Action: "UPDATE", Detail: "set agent.system_prompt_file"})
	}
	if generatedChanged {
		changes = append(changes, Change{Path: GeneratedPromptRel, Action: "WRITE", Detail: "Base → User → OMR composed Prompt"})
	}
	if profilesChanged {
		for _, profile := range profiles {
			changes = append(changes, Change{Path: profile.Rel, Action: "WRITE", Detail: "install read-only " + profile.ID + " Profile"})
		}
	}
	if configChanged {
		changes = append(changes, Change{Path: backupRel + "/reasonix.toml", Action: "BACKUP", Detail: "preserve pre-install config"})
	}
	if manifestChanged {
		changes = append(changes, Change{Path: ManifestRel, Action: "WRITE", Detail: "record asset sources and hashes"})
	}
	return changes
}

func writeInstall(root, configPath string, oldConfig []byte, newConfig string, generatedPath string, generated []byte, profiles []profileAsset, manifestPath string, m manifest.Manifest, backupRel string, configChanged, generatedChanged, profilesChanged, manifestChanged bool) error {
	oldGenerated, generatedExisted := readIfExists(generatedPath)
	oldProfiles := map[string][]byte{}
	profileExisted := map[string]bool{}
	for _, profile := range profiles {
		path := ProfilePath(root, profile.Rel)
		oldProfiles[profile.Rel], profileExisted[profile.Rel] = readIfExists(path)
	}
	oldManifest, manifestExisted := readIfExists(manifestPath)
	backupPath := filepath.Join(root, filepath.FromSlash(backupRel), "reasonix.toml")
	backupCreated := false
	rollback := func() {
		if configChanged {
			restoreFile(configPath, true, oldConfig)
		}
		if generatedChanged {
			restoreFile(generatedPath, generatedExisted, oldGenerated)
		}
		if profilesChanged {
			for _, profile := range profiles {
				restoreFile(ProfilePath(root, profile.Rel), profileExisted[profile.Rel], oldProfiles[profile.Rel])
			}
		}
		if manifestChanged {
			restoreFile(manifestPath, manifestExisted, oldManifest)
		}
		if backupCreated {
			_ = os.Remove(backupPath)
		}
	}
	if configChanged && !fileExists(backupPath) {
		if err := fileutil.AtomicWrite(backupPath, oldConfig, 0o644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
		backupCreated = true
	}
	if generatedChanged {
		if err := fileutil.AtomicWrite(generatedPath, generated, 0o644); err != nil {
			rollback()
			return fmt.Errorf("write generated Prompt: %w", err)
		}
	}
	if profilesChanged {
		for _, profile := range profiles {
			if err := fileutil.AtomicWrite(ProfilePath(root, profile.Rel), profile.Data, 0o644); err != nil {
				rollback()
				return fmt.Errorf("write %s Profile: %w", profile.ID, err)
			}
		}
	}
	if configChanged {
		if err := fileutil.AtomicWrite(configPath, []byte(newConfig), 0o644); err != nil {
			rollback()
			return fmt.Errorf("write reasonix.toml: %w", err)
		}
	}
	if manifestChanged {
		if err := manifest.Write(manifestPath, m); err != nil {
			rollback()
			return fmt.Errorf("write manifest: %w", err)
		}
	}
	return nil
}

func readIfExists(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	return data, err == nil
}

func restoreFile(path string, existed bool, data []byte) {
	if existed {
		_ = fileutil.AtomicWrite(path, data, 0o644)
	} else {
		_ = os.Remove(path)
	}
}

func loadManifest(root string) (manifest.Manifest, bool, error) {
	path := ManifestPath(root)
	m, err := manifest.Load(path)
	if err == nil {
		return m, true, nil
	}
	if os.IsNotExist(err) {
		return manifest.Manifest{}, false, nil
	}
	return manifest.Manifest{}, false, err
}

func conflictReport(root, message string) (Report, error) {
	r := Report{Root: root, Conflicts: []string{message}}
	return r, fmt.Errorf("%s", message)
}

func valueOrEmpty(value tomlValue) string {
	if !value.Present {
		return ""
	}
	return value.Value
}

func stringPointerIfPresent(value tomlValue) *string {
	if !value.Present {
		return nil
	}
	copy := value.Value
	return &copy
}

func fileHashDiffers(path, expected string) bool {
	actual, err := fileutil.SHA256File(path)
	return err != nil || actual != expected
}

// PromptSourceDrift compares the installed Manifest's source hashes with the
// current asset files and, when applicable, the user's source Prompt. It
// returns human-readable diagnostics without exposing Prompt bodies.
func PromptSourceDrift(root string, m manifest.Manifest, assets Assets) []string {
	var drift []string
	if len(assets.BasePrompt) > 0 {
		actual := promptcompose.SHA256String(promptcompose.Canonicalize(string(assets.BasePrompt)))
		if actual != m.Prompt.BaseSHA256 {
			drift = append(drift, "Reasonix base Prompt source hash changed")
		}
	}
	if len(assets.Orchestrator) > 0 {
		actual := promptcompose.SHA256String(promptcompose.Canonicalize(string(assets.Orchestrator)))
		if actual != m.Prompt.OrchestratorSHA256 {
			drift = append(drift, "OMR Orchestrator Prompt source hash changed")
		}
	}
	if !m.Prompt.UserPresent {
		return drift
	}
	configPath := filepath.Join(root, "reasonix.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return append(drift, "cannot read reasonix.toml to check User Prompt source")
	}
	cfg := parseAgentConfig(string(configData))
	var value string
	if cfg.SystemPrompt.Present {
		value = cfg.SystemPrompt.Value
	} else if m.Prompt.UserSource != "" && m.Prompt.UserSource != "inline" {
		value, err = readPromptSource(root, m.Prompt.UserSource)
	}
	if err != nil {
		return append(drift, "User Prompt source is missing")
	}
	if promptcompose.Canonicalize(value) == "" || promptcompose.SHA256String(promptcompose.Canonicalize(value)) != m.Prompt.UserSHA256 {
		drift = append(drift, "User Prompt source hash changed")
	}
	return drift
}

func manifestsEqual(a, b manifest.Manifest) bool {
	return a.SchemaVersion == b.SchemaVersion && a.Product == b.Product && a.Version == b.Version && a.ReasonixCommit == b.ReasonixCommit && a.Prompt == b.Prompt && a.ProfilePath == b.ProfilePath && a.ProfileSHA256 == b.ProfileSHA256 && a.BackupPath == b.BackupPath && equalProfiles(a.Profiles, b.Profiles) && equalConfig(a.Config, b.Config) && equalAssets(a.Assets, b.Assets)
}

func equalProfiles(a, b []manifest.Profile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalConfig(a, b []manifest.ConfigEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].InstalledValue != b[i].InstalledValue || !samePointer(a[i].BaseValue, b[i].BaseValue) {
			return false
		}
	}
	return true
}

func equalAssets(a, b []manifest.Asset) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func samePointer(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func trimPromptSource(source string) string {
	return strings.TrimSpace(source)
}

// getOrphanEventPaths scans .reasonix/omr/sessions/ for .event-index.json files
// that are not expected in a fresh install. Their presence indicates leftover
// session artifacts that may conflict with OMR management.
func getOrphanEventPaths(root string) []string {
	dir := filepath.Join(root, filepath.FromSlash(".reasonix/omr/sessions"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var orphans []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".event-index.json") {
			orphans = append(orphans, entry.Name())
		}
	}
	return orphans
}
