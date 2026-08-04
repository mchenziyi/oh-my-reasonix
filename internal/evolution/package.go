package evolution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

const (
	PackageFormatV1 = "omr-evolution-proposals-v1"
	PackageFormatV2 = "omr-evolution-proposals-v2"
	PackageFormat   = PackageFormatV2
)

const (
	MaxPackageBytes     = 1 << 20
	MaxPackageProposals = 100
)

// SignatureAlgorithmEd25519 is the default local signing algorithm.
const SignatureAlgorithmEd25519 = "ed25519"

type ExperiencePackage struct {
	SchemaVersion      int        `json:"schema_version"`
	Format             string     `json:"format"`
	SourceScopeID      string     `json:"source_scope_id"`
	CreatedAt          string     `json:"created_at"`
	OMRVersion         string     `json:"omr_version,omitempty"`
	ToolVersion        string     `json:"tool_version,omitempty"`
	ProposalCount      int        `json:"proposal_count,omitempty"`
	SignatureAlgorithm string     `json:"signature_algorithm,omitempty"`
	PublicKey          string     `json:"public_key,omitempty"`
	Signature          string     `json:"signature,omitempty"`
	Proposals          []Proposal `json:"proposals"`
	SHA256             string     `json:"sha256"`
}

// IsSigned reports whether the package carries a signature.
func (p ExperiencePackage) IsSigned() bool { return p.Signature != "" && p.SignatureAlgorithm != "" }

// signablePayload is the canonical byte range covered by both the content
// hash and the signature: the whole package JSON except the self-referential
// sha256 and signature fields. It therefore binds metadata, proposals, and
// the embedded public key together.
func (p ExperiencePackage) signablePayload() []byte {
	p.SHA256 = ""
	p.Signature = ""
	b, _ := json.Marshal(p)
	return b
}

func packageHash(p ExperiencePackage) string {
	h := sha256.Sum256(p.signablePayload())
	return hex.EncodeToString(h[:])
}

func (p ExperiencePackage) Validate() error {
	if p.SchemaVersion != SchemaVersion || p.SourceScopeID == "" || p.CreatedAt == "" {
		return fmt.Errorf("invalid evolution package")
	}
	if p.Format != PackageFormatV1 && p.Format != PackageFormatV2 {
		return fmt.Errorf("unsupported evolution package format %q", p.Format)
	}
	if p.SHA256 == "" || packageHash(p) != p.SHA256 {
		return fmt.Errorf("evolution package hash mismatch")
	}
	if p.IsSigned() {
		if p.SignatureAlgorithm != SignatureAlgorithmEd25519 {
			return fmt.Errorf("unsupported signature algorithm %q", p.SignatureAlgorithm)
		}
		if p.PublicKey == "" {
			return fmt.Errorf("signed package missing public key")
		}
		if !p.VerifySignature() {
			return fmt.Errorf("evolution package signature mismatch")
		}
	}
	for _, proposal := range p.Proposals {
		if err := proposal.Validate(); err != nil {
			return fmt.Errorf("invalid packaged proposal: %w", err)
		}
	}
	return nil
}

// LoadPackage reads and fully validates a package file (hash + signature).
func LoadPackage(path string) (ExperiencePackage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ExperiencePackage{}, err
	}
	if len(b) > MaxPackageBytes {
		return ExperiencePackage{}, fmt.Errorf("evolution package exceeds size limit")
	}
	var p ExperiencePackage
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return ExperiencePackage{}, fmt.Errorf("invalid evolution package JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return ExperiencePackage{}, err
	}
	return p, nil
}

type ExportOptions struct {
	Sign       bool
	PrivateKey []byte // PEM or raw ed25519 private key bytes
}

// ExportPackage exports an unsigned package (backward-compatible).
func ExportPackage(store Store, path string) error {
	return ExportPackageWithOptions(store, path, ExportOptions{})
}

// ExportPackageWithOptions exports a package, optionally signing it with a
// local Ed25519 key. The private key never leaves the caller's machine and is
// never written into the project or the package.
func ExportPackageWithOptions(store Store, path string, options ExportOptions) error {
	proposals, err := store.ListProposals()
	if err != nil {
		return err
	}
	if len(proposals) > MaxPackageProposals {
		return fmt.Errorf("too many proposals")
	}
	p := ExperiencePackage{
		SchemaVersion: SchemaVersion,
		Format:        PackageFormat,
		SourceScopeID: store.ScopeID,
		CreatedAt:     Now(),
		OMRVersion:    manifest.Version,
		ToolVersion:   "omr-cli",
		ProposalCount: len(proposals),
		Proposals:     proposals,
	}
	if options.Sign {
		if len(options.PrivateKey) == 0 {
			return fmt.Errorf("signing requires a private key")
		}
		signed, err := signPackage(p, options.PrivateKey)
		if err != nil {
			return err
		}
		return writePackage(signed, path)
	}
	p.SHA256 = packageHash(p)
	return writePackage(p, path)
}

func writePackage(p ExperiencePackage, path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, b, 0600)
}

type ImportOptions struct {
	DryRun           bool
	RequireSignature bool
	TrustedKey       []byte // optional PEM/raw ed25519 public key
}

type ImportResult struct {
	Imported        int      `json:"imported"`
	Skipped         int      `json:"skipped"`
	Conflicts       []string `json:"conflicts,omitempty"`
	DryRun          bool     `json:"dry_run"`
	UnsignedWarning bool     `json:"unsigned_warning,omitempty"`
}

func ImportPackage(store Store, path string) (int, error) {
	result, err := ImportPackageWithOptions(store, path, ImportOptions{})
	return result.Imported, err
}

func ImportPackageWithOptions(store Store, path string, options ImportOptions) (ImportResult, error) {
	result := ImportResult{DryRun: options.DryRun}
	p, err := LoadPackage(path)
	if err != nil {
		return result, err
	}
	// Signature policy.
	if options.RequireSignature {
		if !p.IsSigned() {
			return result, fmt.Errorf("experience package is not signed (--require-signature)")
		}
		if len(options.TrustedKey) > 0 && !verifyAgainstTrustedKey(p, options.TrustedKey) {
			return result, fmt.Errorf("experience package signature does not match --trusted-key")
		}
	} else if !p.IsSigned() {
		result.UnsignedWarning = true
	}
	if len(p.Proposals) > MaxPackageProposals {
		return result, fmt.Errorf("too many proposals")
	}
	existing, err := store.ListProposals()
	if err != nil {
		return result, err
	}
	bySource := map[string]Proposal{}
	for _, proposal := range existing {
		if proposal.ImportedFrom != "" {
			bySource[proposal.ImportedFrom] = proposal
		}
	}
	locals := make([]Proposal, 0, len(p.Proposals))
	for _, source := range p.Proposals {
		local := source
		local.ID = NewID("proposal", store.ScopeID+"|"+p.SourceScopeID+"|"+source.ID)
		local.PatternID = NewID("pattern", store.ScopeID+"|"+p.SourceScopeID+"|"+source.PatternID)
		local.Status = "pending"
		local.ApprovedAt = ""
		local.RollbackReason = ""
		local.ImportedFrom = p.SourceScopeID + ":" + source.ID
		local.EvidenceCount = source.EvidenceCount
		local.CreatedAt = Now()
		local.UpdatedAt = local.CreatedAt
		key := local.ImportedFrom
		if previous, ok := bySource[key]; ok {
			if previous.ContentSHA256 == local.ContentSHA256 {
				result.Skipped++
				continue
			}
			result.Conflicts = append(result.Conflicts, key)
			continue
		}
		locals = append(locals, local)
	}
	if len(result.Conflicts) > 0 {
		return result, fmt.Errorf("experience package conflicts: %v", result.Conflicts)
	}
	if options.DryRun {
		result.Imported = len(locals)
		return result, nil
	}
	for _, local := range locals {
		if err := store.SaveProposal(local); err != nil {
			return result, err
		}
		result.Imported++
	}
	return result, nil
}
