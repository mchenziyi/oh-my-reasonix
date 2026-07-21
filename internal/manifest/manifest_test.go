package manifest

import "testing"

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
