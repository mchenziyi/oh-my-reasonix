package manifest

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

const (
	SchemaVersion  = 1
	Product        = "oh-my-reasonix"
	Version        = "2.0.0"
	ReasonixCommit = "464d494"
)

type Asset struct {
	ID               string `json:"id"`
	Role             string `json:"role"`
	SourceProject    string `json:"source_project"`
	SourceVersion    string `json:"source_version"`
	SourceCommit     string `json:"source_commit"`
	SourcePath       string `json:"source_path"`
	LicenseStatus    string `json:"license_status"`
	ContentSHA256    string `json:"content_sha256"`
	InstallTarget    string `json:"install_target"`
	CompositionOrder int    `json:"composition_order,omitempty"`
}

type ConfigEntry struct {
	Path           string  `json:"path"`
	BaseValue      *string `json:"base_value"`
	InstalledValue string  `json:"installed_value"`
}

type Prompt struct {
	GeneratedPath      string `json:"generated_path"`
	BaseSource         string `json:"base_source"`
	BaseSHA256         string `json:"base_sha256"`
	UserPresent        bool   `json:"user_present"`
	UserSource         string `json:"user_source,omitempty"`
	UserSHA256         string `json:"user_sha256,omitempty"`
	OrchestratorSource string `json:"orchestrator_source"`
	OrchestratorSHA256 string `json:"orchestrator_sha256"`
	FinalSHA256        string `json:"final_sha256"`
}

type Profile struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	ContentSHA256 string `json:"content_sha256"`
}

type Manifest struct {
	SchemaVersion  int              `json:"schema_version"`
	Product        string           `json:"product"`
	Version        string           `json:"version"`
	ReasonixCommit string           `json:"reasonix_commit"`
	Prompt         Prompt           `json:"prompt"`
	Assets         []Asset          `json:"assets"`
	Config         []ConfigEntry    `json:"config"`
	Profiles       []Profile        `json:"profiles,omitempty"`
	ProfilePath    string           `json:"profile_path,omitempty"`
	ProfileSHA256  string           `json:"profile_sha256,omitempty"`
	BackupPath     string           `json:"backup_path,omitempty"`
	Hook           *HookRecord      `json:"hook,omitempty"`
	Evolution      *EvolutionRecord `json:"evolution,omitempty"`
}

type EvolutionRecord struct {
	Enabled       bool   `json:"enabled"`
	OverlayPath   string `json:"overlay_path,omitempty"`
	OverlaySHA256 string `json:"overlay_sha256,omitempty"`
}

// HookRecord records the OMR-managed Hook state in the Manifest.
// It is entirely optional; old manifests without it remain valid.
type HookRecord struct {
	Enabled             bool   `json:"enabled"`
	SettingsPath        string `json:"settings_path"`
	Event               string `json:"event"`
	Description         string `json:"description"`
	EntrySHA256         string `json:"entry_sha256"`
	BaseFileSHA256      string `json:"base_file_sha256,omitempty"`
	InstalledFileSHA256 string `json:"installed_file_sha256,omitempty"`
}

func New() Manifest {
	return Manifest{
		SchemaVersion:  SchemaVersion,
		Product:        Product,
		Version:        Version,
		ReasonixCommit: ReasonixCommit,
	}
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported manifest schema_version %d", m.SchemaVersion)
	}
	if m.Product != Product {
		return fmt.Errorf("unexpected manifest product %q", m.Product)
	}
	if m.Prompt.GeneratedPath == "" || m.Prompt.FinalSHA256 == "" {
		return fmt.Errorf("manifest prompt metadata is incomplete")
	}
	if len(m.NormalizedProfiles()) == 0 {
		return fmt.Errorf("manifest profile metadata is incomplete")
	}
	for _, profile := range m.NormalizedProfiles() {
		if profile.ID == "" || profile.Path == "" || profile.ContentSHA256 == "" {
			return fmt.Errorf("manifest profile metadata is incomplete")
		}
	}
	for _, asset := range m.Assets {
		status := strings.ToLower(strings.TrimSpace(asset.LicenseStatus))
		if status == "" || status == "unknown" || status == "未确认" {
			return fmt.Errorf("asset %q has unresolved license status", asset.ID)
		}
	}
	if m.Hook != nil {
		if m.Hook.SettingsPath != ".reasonix/settings.json" ||
			m.Hook.Event != "PreToolUse" ||
			m.Hook.Description != "[oh-my-reasonix] Comment Checker before git commit" {
			return fmt.Errorf("manifest Hook metadata is incomplete")
		}
		if !validSHA256(m.Hook.EntrySHA256) {
			return fmt.Errorf("manifest Hook entry hash is invalid")
		}
		if m.Hook.Enabled && !validSHA256(m.Hook.InstalledFileSHA256) {
			return fmt.Errorf("manifest Hook installed file hash is invalid")
		}
		for _, value := range []string{m.Hook.BaseFileSHA256, m.Hook.InstalledFileSHA256} {
			if value != "" && !validSHA256(value) {
				return fmt.Errorf("manifest Hook file hash is invalid")
			}
		}
		if !m.Hook.Enabled && (m.Hook.BaseFileSHA256 != "" || m.Hook.InstalledFileSHA256 != "") {
			return fmt.Errorf("disabled manifest Hook contains enabled-state file hashes")
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (m Manifest) NormalizedProfiles() []Profile {
	if len(m.Profiles) > 0 {
		return append([]Profile(nil), m.Profiles...)
	}
	if m.ProfilePath == "" || m.ProfileSHA256 == "" {
		return nil
	}
	return []Profile{{ID: "omr-explore", Path: m.ProfilePath, ContentSHA256: m.ProfileSHA256}}
}

// Write stores JSON, which is a valid YAML 1.2 document, under the required
// .yaml filename. Keeping the representation dependency-free makes the CLI
// portable while retaining a machine-readable manifest.
func Write(path string, m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return fileutil.AtomicWrite(path, b, 0o644)
}

func Load(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
