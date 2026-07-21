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
