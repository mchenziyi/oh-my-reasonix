package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MEM-01D transaction record documents. All paths under transactions/,
// idempotency/, generations/ and CURRENT reuse the FactStore secure path and
// atomic-write machinery; these files are audit records and derived state,
// not second fact sources.

// txStatus is the persisted transaction state, derived from which record
// files exist (prepared.json is always present; commit.json/abort.json mark
// the terminal states).
type txStatus string

const (
	txPending   txStatus = "pending"
	txCommitted txStatus = "committed"
	txAborted   txStatus = "aborted"
)

// txRecord is the content of transactions/<txID>/prepared.json.
type txRecord struct {
	SchemaVersion           int            `json:"schema_version"`
	TransactionID           string         `json:"transaction_id"`
	IdempotencyKey          string         `json:"idempotency_key"`
	GenerationID            string         `json:"generation_id"`
	Scope                   Scope          `json:"scope"`
	BaseGeneration          *string        `json:"base_generation"`
	CompilerVersion         string         `json:"compiler_version"`
	CanonicalizationVersion int            `json:"canonicalization_version"`
	RequestSHA256           string         `json:"request_sha256"`
	PreparedFacts           []preparedFact `json:"prepared_facts"`
	CreatedAt               string         `json:"created_at"`
}

// preparedFact pins the structured identity of one staged fact so the
// Manifest inputs can be verified item by item at Commit time: fact type and
// id, schema version, scope and content hash must all match exactly, and the
// fact scope must match the transaction scope. The canonical bytes remain
// for audit; they are never trusted for identity.
type preparedFact struct {
	FactType          string `json:"fact_type"`
	FactID            string `json:"fact_id"`
	FactSchemaVersion int    `json:"fact_schema_version"`
	Scope             Scope  `json:"scope"`
	ContentSHA256     string `json:"content_sha256"`
	Canonical         []byte `json:"canonical"`
}

// factIdentity derives the stable manifest reference (fact_type + fact_id)
// of a fact, in the Architecture v1 format ("memory_revision"/"mem_abc@2",
// "memory_evidence_generation"/"mem_abc@2:evidence@3"). The derivation and
// its inverse (resolveManifestInput) are symmetric so prepared and committed
// facts verify against the same identity.
func factIdentity(f Fact) (factType, factID string, err error) {
	switch v := f.(type) {
	case MemoryRevision:
		return "memory_revision", fmt.Sprintf("%s@%d", v.MemoryID, v.Revision), nil
	case MemoryEvidenceGeneration:
		return "memory_evidence_generation",
			fmt.Sprintf("%s@%d:evidence@%d", v.MemoryID, v.Revision, v.EvidenceGeneration), nil
	case JudgmentFact:
		return "judgment", v.JudgmentID, nil
	case PolicyFact:
		return "policy", fmt.Sprintf("%s@%d", v.PolicyID, v.PolicyVersion), nil
	case GovernanceEvent:
		return "governance_event", v.EventID, nil
	case GenerationInputManifest:
		return "generation_input_manifest", v.GenerationID, nil
	case RetrievalEvaluation:
		return "retrieval_evaluation", v.EvaluationID, nil
	default:
		return "", "", fmt.Errorf("unsupported fact type %T", f)
	}
}

// factSchemaVersion returns the schema version of a fact document.
func factSchemaVersion(f Fact) int {
	switch v := f.(type) {
	case MemoryRevision:
		return v.SchemaVersion
	case MemoryEvidenceGeneration:
		return v.SchemaVersion
	case JudgmentFact:
		return v.SchemaVersion
	case PolicyFact:
		return v.SchemaVersion
	case GovernanceEvent:
		return v.SchemaVersion
	case GenerationInputManifest:
		return v.SchemaVersion
	case RetrievalEvaluation:
		return v.SchemaVersion
	default:
		return 0
	}
}

// resolveManifestInput maps a manifest (fact_type, fact_id) pair back onto a
// storable fact identity (FactKind + store key) so committed facts can be
// read through the full verification chain. Unknown types and malformed ids
// are rejected; this is the inverse of factIdentity.
func resolveManifestInput(factType, factID string) (FactKind, string, error) {
	if err := validateID(factType, "fact_type"); err != nil {
		return "", "", err
	}
	if err := validateID(factID, "fact_id"); err != nil {
		return "", "", err
	}
	atoi := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	switch factType {
	case "memory_revision":
		idx := strings.LastIndex(factID, "@")
		if idx <= 0 {
			return "", "", fmt.Errorf("malformed memory_revision fact id")
		}
		rev, ok := atoi(factID[idx+1:])
		if !ok {
			return "", "", fmt.Errorf("malformed memory_revision fact id")
		}
		return FactKindMemoryRevision, fmt.Sprintf("%s/%d", factID[:idx], rev), nil
	case "memory_evidence_generation":
		parts := strings.Split(factID, ":evidence@")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("malformed memory_evidence_generation fact id")
		}
		gen, ok := atoi(parts[1])
		if !ok {
			return "", "", fmt.Errorf("malformed memory_evidence_generation fact id")
		}
		idx := strings.LastIndex(parts[0], "@")
		if idx <= 0 {
			return "", "", fmt.Errorf("malformed memory_evidence_generation fact id")
		}
		rev, ok := atoi(parts[0][idx+1:])
		if !ok {
			return "", "", fmt.Errorf("malformed memory_evidence_generation fact id")
		}
		return FactKindMemoryEvidenceGeneration,
			fmt.Sprintf("%s/%d/%d", parts[0][:idx], rev, gen), nil
	case "judgment":
		return FactKindJudgment, factID, nil
	case "policy":
		idx := strings.LastIndex(factID, "@")
		if idx <= 0 {
			return "", "", fmt.Errorf("malformed policy fact id")
		}
		ver, ok := atoi(factID[idx+1:])
		if !ok {
			return "", "", fmt.Errorf("malformed policy fact id")
		}
		return FactKindPolicy, fmt.Sprintf("%s/%d", factID[:idx], ver), nil
	case "governance_event":
		return FactKindGovernanceEvent, factID, nil
	case "generation_input_manifest":
		return FactKindGenerationInputManifest, factID, nil
	case "retrieval_evaluation":
		return FactKindRetrievalEvaluation, factID, nil
	default:
		return "", "", fmt.Errorf("unknown fact type %q", factType)
	}
}

// sameBase compares two base-generation references strictly: nil only equals
// nil, and a non-nil base equals exactly the same value.
func sameBase(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// txCommitRecord is transactions/<txID>/commit.json.
type txCommitRecord struct {
	SchemaVersion            int    `json:"schema_version"`
	TransactionID            string `json:"transaction_id"`
	GenerationID             string `json:"generation_id"`
	BaseGeneration           string `json:"base_generation"`
	OutputGenerationSHA256   string `json:"output_generation_sha256"`
	GenerationManifestSHA256 string `json:"generation_input_manifest_sha256"`
	CommittedAt              string `json:"committed_at"`
}

// txAbortRecord is transactions/<txID>/abort.json.
type txAbortRecord struct {
	SchemaVersion int    `json:"schema_version"`
	TransactionID string `json:"transaction_id"`
	GenerationID  string `json:"generation_id"`
	Reason        string `json:"reason"`
	AbortedAt     string `json:"aborted_at"`
}

// idempotencyClaim is idempotency/<key>.json. It is created atomically
// (no-overwrite) before any business side effect and updated in place by its
// owning transaction; a conflicting writer can never overwrite it.
type idempotencyClaim struct {
	SchemaVersion  int      `json:"schema_version"`
	IdempotencyKey string   `json:"idempotency_key"`
	RequestSHA256  string   `json:"request_sha256"`
	TransactionID  string   `json:"transaction_id"`
	GenerationID   string   `json:"generation_id"`
	Status         txStatus `json:"status"`
	CreatedAt      string   `json:"created_at"`
}

// currentPointer is the root CURRENT document: the single effective commit
// point. Its generation_id is the only Generation that normal reads adopt.
type currentPointer struct {
	SchemaVersion          int    `json:"schema_version"`
	GenerationID           string `json:"generation_id"`
	OutputGenerationSHA256 string `json:"output_generation_sha256"`
	TransactionID          string `json:"transaction_id"`
	CreatedAt              string `json:"created_at"`
}

// generationDoc is generations/<id>/generation.json. It deliberately does
// not reference the Manifest and carries no created_at: the output
// generation_sha256 is the deterministic hash of the other fields, so a
// caller can predict it while preparing the Manifest. CompiledOutputSHA256
// pins the deterministic hash of the compiled OKF views (wiki/ and state/)
// so a deleted derived view set can be rebuilt and verified byte-for-byte;
// it is not part of output_generation_sha256 (which stays stable for the
// document metadata itself).
type generationDoc struct {
	SchemaVersion           int    `json:"schema_version"`
	GenerationID            string `json:"generation_id"`
	Scope                   Scope  `json:"scope"`
	BaseGeneration          any    `json:"base_generation"` // string or nil
	CompilerVersion         string `json:"compiler_version"`
	CanonicalizationVersion int    `json:"canonicalization_version"`
	TransactionID           string `json:"transaction_id"`
	CompiledOutputSHA256    string `json:"compiled_output_sha256,omitempty"`
	OutputGenerationSHA256  string `json:"output_generation_sha256"`
}

// outputHash computes the deterministic content hash of the document,
// excluding the output hash field itself.
func (g generationDoc) outputHash() (string, error) {
	var base any
	if g.BaseGeneration != nil {
		base = g.BaseGeneration
	}
	b, err := json.Marshal(map[string]any{
		"schema_version":           g.SchemaVersion,
		"generation_id":            g.GenerationID,
		"scope":                    string(g.Scope),
		"base_generation":          base,
		"compiler_version":         g.CompilerVersion,
		"canonicalization_version": g.CanonicalizationVersion,
		"transaction_id":           g.TransactionID,
	})
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

// supportedGenerationCompilers is the skeleton compiler registry. Only
// registered compiler + canonicalization version pairs may build staging
// generations or be used for deterministic rebuild; unknown versions block
// with memory_generation_compiler_unavailable instead of guessing.
var supportedGenerationCompilers = map[string]int{
	"mnemosyne-compiler/1": 1,                          // MEM-01D skeleton compiler
	OKFCompilerVersionV1:   OKFCanonicalizationVersion, // MEM-01E legacy compiler
	OKFCompilerVersion:     OKFCanonicalizationVersion, // MEM-01E OKF compiler
}

func generationCompilerAvailable(compiler string, canon int) bool {
	v, ok := supportedGenerationCompilers[compiler]
	return ok && v == canon
}

// requestHash is the deterministic binding of a generation request to its
// idempotency key: the same key with a different request hash fails closed.
func requestHash(scope Scope, base *string, compiler string, canon int) (string, error) {
	var baseVal any
	if base != nil {
		baseVal = *base
	}
	b, err := json.Marshal(map[string]any{
		"scope":                    string(scope),
		"base_generation":          baseVal,
		"compiler_version":         compiler,
		"canonicalization_version": canon,
	})
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func newRandomID(prefix string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// readTxStatus derives the transaction state from the record files present.
func (gs *generationStore) readTxStatus(ctx context.Context, txID string) (txStatus, error) {
	base, err := gs.txDir(ctx, txID)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(base, "commit.json")); err == nil {
		return txCommitted, nil
	}
	if _, err := os.Stat(filepath.Join(base, "abort.json")); err == nil {
		return txAborted, nil
	}
	if _, err := os.Stat(filepath.Join(base, "prepared.json")); err == nil {
		return txPending, nil
	}
	return "", storeError(CodeNotFound, "transaction record not found")
}
