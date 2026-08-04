package evolution

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Signing helpers --------------------------------------------------------

func generateTestKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func writeKeyFile(t *testing.T, path string, key []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, key, mode); err != nil {
		t.Fatal(err)
	}
}

func lp05SourceStore(t *testing.T) (Store, string) {
	t.Helper()
	s, _ := NewStore(t.TempDir())
	overlay := "rule"
	h := sha256.Sum256([]byte(overlay))
	if err := s.SaveProposal(Proposal{SchemaVersion: 1, ID: "p1", PatternID: "pattern-1", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "pending", CreatedAt: Now(), UpdatedAt: Now()}); err != nil {
		t.Fatal(err)
	}
	return s, overlay
}

// --- Metadata & signature ---------------------------------------------------

func TestSignedExportIncludesMetadata(t *testing.T) {
	s, _ := lp05SourceStore(t)
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var pkg ExperiencePackage
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.OMRVersion == "" || pkg.ToolVersion == "" || pkg.SourceScopeID == "" || pkg.ProposalCount != 1 || pkg.SignatureAlgorithm != "ed25519" {
		t.Fatalf("missing metadata: %+v", pkg)
	}
	if pkg.Signature == "" {
		t.Fatal("signed package must carry a signature")
	}
	if pkg.PublicKey == "" {
		t.Fatal("signed package must carry the public key")
	}
	// Hash must cover the payload including metadata.
	if pkg.SHA256 == "" {
		t.Fatal("hash missing")
	}
}

func TestSignedPackageVerifiesWithPublicKey(t *testing.T) {
	s, _ := lp05SourceStore(t)
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	pkg, err := LoadPackage(path)
	if err != nil {
		t.Fatal(err)
	}
	if !pkg.VerifySignature() {
		t.Fatal("signature must verify against embedded public key")
	}
}

func TestTamperedPackageSignatureFails(t *testing.T) {
	s, _ := lp05SourceStore(t)
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	// Tamper with proposal title.
	b, _ := os.ReadFile(path)
	tampered := strings.Replace(string(b), `"title": "t"`, `"title": "evil"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPackage(path); err == nil {
		t.Fatal("expected tamper rejection")
	}
}

func TestSignatureWithWrongKeyFails(t *testing.T) {
	s, _ := lp05SourceStore(t)
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	// A different public key must not verify.
	pkg, _ := LoadPackage(path)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if ed25519.Verify(otherPub, pkg.signablePayload(), mustDecodeHex(t, pkg.Signature)) {
		t.Fatal("wrong key must not verify")
	}
}

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// --- Import policies --------------------------------------------------------

func TestImportUnsignedPackageWarnsWithoutSignatureRequirement(t *testing.T) {
	s, _ := lp05SourceStore(t)
	path := filepath.Join(t.TempDir(), "unsigned.json")
	if err := ExportPackage(s, path); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	result, err := ImportPackageWithOptions(target, path, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("unsigned package should import with warning: %+v", result)
	}
	if !result.UnsignedWarning {
		t.Fatalf("expected unsigned warning: %+v", result)
	}
}

func TestImportRequireSignatureRejectsUnsigned(t *testing.T) {
	s, _ := lp05SourceStore(t)
	path := filepath.Join(t.TempDir(), "unsigned.json")
	if err := ExportPackage(s, path); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	_, err := ImportPackageWithOptions(target, path, ImportOptions{RequireSignature: true})
	if err == nil {
		t.Fatal("unsigned package must be rejected with --require-signature")
	}
}

func TestImportSignedWithTrustedKey(t *testing.T) {
	s, _ := lp05SourceStore(t)
	priv, pub := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	result, err := ImportPackageWithOptions(target, path, ImportOptions{RequireSignature: true, TrustedKey: pub})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("signed import failed: %+v", result)
	}
}

func TestImportTrustedKeyMismatchFailsClosed(t *testing.T) {
	s, _ := lp05SourceStore(t)
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	target, _ := NewStore(t.TempDir())
	_, err := ImportPackageWithOptions(target, path, ImportOptions{RequireSignature: true, TrustedKey: otherPub})
	if err == nil {
		t.Fatal("trusted key mismatch must fail closed")
	}
	// Zero partial writes.
	if props, _ := target.ListProposals(); len(props) != 0 {
		t.Fatal("failed import must not write proposals")
	}
}

func TestImportDryRunZeroWritesForSigned(t *testing.T) {
	s, _ := lp05SourceStore(t)
	priv, pub := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	result, err := ImportPackageWithOptions(target, path, ImportOptions{DryRun: true, RequireSignature: true, TrustedKey: pub})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || !result.DryRun {
		t.Fatalf("unexpected dry-run: %+v", result)
	}
	if props, _ := target.ListProposals(); len(props) != 0 {
		t.Fatal("dry-run must not write")
	}
}

// --- Key safety -------------------------------------------------------------

func TestLoadPrivateKeyRejectsWorldReadable(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "key.pem")
	writeKeyFile(t, path, priv, 0644)
	if _, err := LoadPrivateKey(path); err == nil {
		t.Fatal("world-readable private key must be rejected")
	}
}

func TestLoadPrivateKeyRejectsSymlink(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	target := filepath.Join(t.TempDir(), "real-key.pem")
	writeKeyFile(t, target, priv, 0600)
	link := filepath.Join(t.TempDir(), "link-key.pem")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := LoadPrivateKey(link); err == nil {
		t.Fatal("symlinked private key must be rejected")
	}
}

func TestLoadPrivateKeyGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.pem")
	writeKeyFile(t, path, []byte("not a key"), 0600)
	if _, err := LoadPrivateKey(path); err == nil {
		t.Fatal("garbage key must be rejected")
	}
}

func TestLoadPublicKeyGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pub.pem")
	writeKeyFile(t, path, []byte("not a key"), 0600)
	if _, err := LoadPublicKey(path); err == nil {
		t.Fatal("garbage public key must be rejected")
	}
}

// --- Import safety ----------------------------------------------------------

func TestImportRejectsOversizedPackage(t *testing.T) {
	s, _ := lp05SourceStore(t)
	path := filepath.Join(t.TempDir(), "big.json")
	if err := ExportPackage(s, path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	big := append(b, make([]byte, MaxPackageBytes)...)
	if err := os.WriteFile(path, big, 0600); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	if _, err := ImportPackageWithOptions(target, path, ImportOptions{}); err == nil {
		t.Fatal("oversized package must be rejected")
	}
}

func TestSignedPackageImportDoesNotAutoApprove(t *testing.T) {
	s, _ := lp05SourceStore(t)
	priv, pub := generateTestKeyPair(t)
	path := filepath.Join(t.TempDir(), "signed.json")
	if err := ExportPackageWithOptions(s, path, ExportOptions{Sign: true, PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	target, _ := NewStore(t.TempDir())
	if _, err := ImportPackageWithOptions(target, path, ImportOptions{RequireSignature: true, TrustedKey: pub}); err != nil {
		t.Fatal(err)
	}
	props, _ := target.ListProposals()
	if len(props) != 1 || props[0].Status != "pending" {
		t.Fatalf("import must keep proposals pending: %+v", props)
	}
}
