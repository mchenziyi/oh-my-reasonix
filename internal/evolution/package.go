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
)

const PackageFormat = "omr-evolution-proposals-v1"

const (
	MaxPackageBytes     = 1 << 20
	MaxPackageProposals = 100
)

type ExperiencePackage struct {
	SchemaVersion int        `json:"schema_version"`
	Format        string     `json:"format"`
	SourceScopeID string     `json:"source_scope_id"`
	CreatedAt     string     `json:"created_at"`
	Proposals     []Proposal `json:"proposals"`
	SHA256        string     `json:"sha256"`
}

func packageHash(p ExperiencePackage) string {
	p.SHA256 = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (p ExperiencePackage) Validate() error {
	if p.SchemaVersion != SchemaVersion || p.Format != PackageFormat || p.SourceScopeID == "" || p.CreatedAt == "" {
		return fmt.Errorf("invalid evolution package")
	}
	if p.SHA256 == "" || packageHash(p) != p.SHA256 {
		return fmt.Errorf("evolution package hash mismatch")
	}
	for _, proposal := range p.Proposals {
		if err := proposal.Validate(); err != nil {
			return fmt.Errorf("invalid packaged proposal: %w", err)
		}
	}
	return nil
}

func ExportPackage(store Store, path string) error {
	proposals, err := store.ListProposals()
	if err != nil {
		return err
	}
	p := ExperiencePackage{SchemaVersion: SchemaVersion, Format: PackageFormat, SourceScopeID: store.ScopeID, CreatedAt: Now(), Proposals: proposals}
	if len(proposals) > MaxPackageProposals {
		return fmt.Errorf("too many proposals")
	}
	p.SHA256 = packageHash(p)
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

type ImportOptions struct{ DryRun bool }
type ImportResult struct {
	Imported  int      `json:"imported"`
	Skipped   int      `json:"skipped"`
	Conflicts []string `json:"conflicts,omitempty"`
	DryRun    bool     `json:"dry_run"`
}

func ImportPackage(store Store, path string) (int, error) {
	result, err := ImportPackageWithOptions(store, path, ImportOptions{})
	return result.Imported, err
}

func ImportPackageWithOptions(store Store, path string, options ImportOptions) (ImportResult, error) {
	result := ImportResult{DryRun: options.DryRun}
	b, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if len(b) > MaxPackageBytes {
		return result, fmt.Errorf("evolution package exceeds size limit")
	}
	var p ExperiencePackage
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return result, fmt.Errorf("invalid evolution package JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return result, err
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
