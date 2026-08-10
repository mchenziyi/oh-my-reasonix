package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreErrorCodeAndRedaction(t *testing.T) {
	secret := "/Users/secret-user/.reasonix/omr/memory"
	err := storeError(CodeIdentityConflict, "fact identity conflict")
	if ErrorCode(err) != CodeIdentityConflict {
		t.Errorf("ErrorCode = %q, want %q", ErrorCode(err), CodeIdentityConflict)
	}
	if strings.Contains(err.Error(), secret) {
		t.Error("store error must not leak absolute paths")
	}
	if !strings.Contains(err.Error(), "identity conflict") {
		t.Errorf("store error should carry a useful message: %q", err.Error())
	}
	var se *StoreError
	if !errors.As(err, &se) {
		t.Error("StoreError must be extractable via errors.As")
	}
	if ErrorCode(errors.New("plain error")) != "" {
		t.Error("ErrorCode of non-store error should be empty")
	}
}

func TestHashMismatchClassificationNotFooledByInjection(t *testing.T) {
	// A crafted ID value embedding the hash-mismatch text must be
	// classified as schema_invalid, never hash_mismatch.
	rev := validRevision()
	rev.MemoryID = "x content_sha256 mismatch y"
	if err := rev.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
	if ErrorCode(classifyValidateError(rev.Validate())) != CodeSchemaInvalid {
		t.Error("injected mismatch text must not be classified as hash mismatch")
	}
	// A genuine trailing hash-mismatch error still maps correctly.
	rev2 := validRevision()
	rev2.ContentSHA256 = "sha256_" + strings.Repeat("f", 64)
	if err := rev2.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
	if ErrorCode(classifyValidateError(rev2.Validate())) != CodeHashMismatch {
		t.Error("genuine hash mismatch must be classified as hash_mismatch")
	}
}

func TestDiagnoseReportsResiduals(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	diags, err := s.Diagnose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Errorf("fresh store should have no diagnostics, got %v", diags)
	}

	// Residual temp file must be reported (not silently deleted).
	if err := writeFileForTest(root, "facts/judgments/.omr-fact-abcd.tmp", []byte("partial")); err != nil {
		t.Fatal(err)
	}
	diags, err = s.Diagnose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || diags[0].Code != CodeCorruptFile {
		t.Errorf("expected residual temp diagnostic, got %v", diags)
	}
	if strings.Contains(diags[0].Detail, root) {
		t.Error("diagnostic detail must be redacted")
	}
}

func writeFileForTest(root, rel string, data []byte) error {
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o600)
}
