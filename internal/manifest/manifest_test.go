package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validManifest() Manifest {
	m := New()
	m.Prompt.GeneratedPath = "generated.md"
	m.Prompt.FinalSHA256 = "prompt-hash"
	m.ProfilePath = "profile/SKILL.md"
	m.ProfileSHA256 = "profile-hash"
	m.Assets = []Asset{{ID: "owned", LicenseStatus: "project-owned"}}
	return m
}

func TestValidateRejectsUnresolvedLicense(t *testing.T) {
	for _, status := range []string{"", "unknown", "未确认"} {
		m := validManifest()
		m.Assets[0].LicenseStatus = status
		if err := m.Validate(); err == nil {
			t.Fatalf("expected unresolved license %q to fail", status)
		}
	}
}

func TestValidateAcceptsKnownLicense(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizedProfilesAcceptsLegacyFields(t *testing.T) {
	profiles := validManifest().NormalizedProfiles()
	if len(profiles) != 1 || profiles[0].ID != "omr-explore" || profiles[0].Path != "profile/SKILL.md" || profiles[0].ContentSHA256 != "profile-hash" {
		t.Fatalf("unexpected normalized profiles: %#v", profiles)
	}
}

func TestValidateAcceptsProfilesList(t *testing.T) {
	m := New()
	m.Prompt.GeneratedPath = "generated.md"
	m.Prompt.FinalSHA256 = "prompt-hash"
	m.Profiles = []Profile{{ID: "omr-explore", Path: "profile/SKILL.md", ContentSHA256: "profile-hash"}}
	m.Assets = []Asset{{ID: "owned", LicenseStatus: "project-owned"}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsIncompleteProfile(t *testing.T) {
	m := validManifest()
	m.ProfilePath = ""
	m.ProfileSHA256 = ""
	m.Profiles = []Profile{{ID: "omr-explore", Path: "profile/SKILL.md"}}
	if err := m.Validate(); err == nil {
		t.Fatal("expected incomplete profile to fail")
	}
}

func TestWriteAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.lock.yaml")
	m := validManifest()
	m.Config = []ConfigEntry{{Path: "agent.system_prompt_file", InstalledValue: "generated.md"}}
	if err := Write(path, m); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Product != Product || loaded.Prompt.GeneratedPath != m.Prompt.GeneratedPath || len(loaded.Config) != 1 {
		t.Fatalf("unexpected round trip manifest: %#v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("manifest should end with newline: err=%v data=%q", err, data)
	}
}

func TestLoadRejectsMalformedManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.lock.yaml")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "parse manifest") {
		t.Fatalf("expected malformed manifest error, got %v", err)
	}
}

func TestWriteRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.lock.yaml")
	if err := Write(path, Manifest{}); err == nil {
		t.Fatal("expected invalid manifest to be rejected")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid manifest should not be written: %v", err)
	}
}
