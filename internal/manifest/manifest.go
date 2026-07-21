package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

const (
	SchemaVersion  = 1
	Product        = "oh-my-reasonix"
	Version        = "1.1.1"
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

type Manifest struct {
	SchemaVersion  int           `json:"schema_version"`
	Product        string        `json:"product"`
	Version        string        `json:"version"`
	ReasonixCommit string        `json:"reasonix_commit"`
	Prompt         Prompt        `json:"prompt"`
	Assets         []Asset       `json:"assets"`
	Config         []ConfigEntry `json:"config"`
	ProfilePath    string        `json:"profile_path"`
	ProfileSHA256  string        `json:"profile_sha256"`
	BackupPath     string        `json:"backup_path,omitempty"`
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
	if m.ProfilePath == "" || m.ProfileSHA256 == "" {
		return fmt.Errorf("manifest profile metadata is incomplete")
	}
	for _, asset := range m.Assets {
		status := strings.ToLower(strings.TrimSpace(asset.LicenseStatus))
		if status == "" || status == "unknown" || status == "未确认" {
			return fmt.Errorf("asset %q has unresolved license status", asset.ID)
		}
	}
	return nil
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
