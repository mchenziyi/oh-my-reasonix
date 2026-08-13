package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// ---- derived state vocabulary ----

// Lifecycle is the derived promotion state of a memory. It is a pure
// function of the Usage Policy, its evidence facts, and the Governance
// Event chain — never a second fact source.
type Lifecycle string

const (
	LifecycleProbation  Lifecycle = "probation"
	LifecycleActive     Lifecycle = "active"
	LifecycleDegraded   Lifecycle = "degraded"
	LifecycleFrozen     Lifecycle = "frozen"
	LifecycleSuperseded Lifecycle = "superseded"
	LifecycleArchived   Lifecycle = "archived"
)

// Health is a derived dimension independent of Lifecycle and Freshness.
type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
)

// Freshness is a derived dimension independent of Lifecycle/Health. Time
// passing can only ever produce fresh/aging/needs_revalidation — it never
// produces frozen, superseded or archived.
type Freshness string

const (
	FreshnessFresh             Freshness = "fresh"
	FreshnessAging             Freshness = "aging"
	FreshnessNeedsRevalidation Freshness = "needs_revalidation"
)

// UsageStats aggregates the MemoryUsage/Outcome/AttributionOverride facts of
// one memory revision. external_failure outcomes count as usage but never as
// help/harm; a usage without any attributable outcome is flagged
// insufficient_evidence instead of guessing an effect.
type UsageStats struct {
	UsageCount           int    `json:"usage_count"`
	CountedHelpCount     int    `json:"counted_help_count"`
	CountedHarmCount     int    `json:"counted_harm_count"`
	LastUsedAt           string `json:"last_used_at"`
	InsufficientEvidence bool   `json:"insufficient_evidence"`
}

// DerivedMemoryState is the derived snapshot of one memory revision. It is
// reconstructible from the canonical facts at any time and never written
// back into a Revision.
type DerivedMemoryState struct {
	Scope         Scope       `json:"scope"`
	MemoryID      string      `json:"memory_id"`
	MemoryType    MemoryType  `json:"memory_type"`
	CanonicalKey  string      `json:"canonical_key"`
	Revision      int         `json:"revision"`
	ContentSHA256 string      `json:"content_sha256"`
	UsagePolicy   UsagePolicy `json:"usage_policy"`
	Lifecycle     Lifecycle   `json:"lifecycle"`
	Health        Health      `json:"health"`
	Freshness     Freshness   `json:"freshness"`
	Pinned        bool        `json:"pinned"`
	Frozen        bool        `json:"frozen"`
	Archived      bool        `json:"archived"`
	Usage         UsageStats  `json:"usage"`
}

// SnapshotBytes is the deterministic, read-only serialization of the derived
// snapshot, intended as the OKF compiler's DerivedMemoryState input adapter
// (MEM-01F-05). Any fact change that alters the derived state changes these
// bytes, so a compiler consuming them sees a deriveable input-hash change.
func (d DerivedMemoryState) SnapshotBytes() []byte {
	b, err := json.Marshal(d)
	if err != nil {
		// All fields are primitive scalars; marshal cannot fail.
		return []byte{}
	}
	return b
}

// DerivedStateRequest pins every input the derivation may depend on: the
// scope (must match the store), the fixed evaluation time point (so results
// are deterministic for a given time), the Freshness policy and the Index
// policy. nil policies fall back to the frozen defaults below.
type DerivedStateRequest struct {
	Scope           Scope
	Now             time.Time
	Revision        int // 0 = latest revision of each memory; >0 pins one revision (superseded when not latest)
	FreshnessPolicy *PolicyConfigFreshness
	IndexPolicy     *PolicyConfigIndex
}

// DerivedStateResult carries the sorted derived states and the three
// deterministic index documents. GlobalIndex is only populated when the
// store scope is global; a project store derives an empty document (the two
// scopes are isolated and a project store never reads global facts).
type DerivedStateResult struct {
	States      []DerivedMemoryState `json:"states"`
	RootIndex   RootIndexDoc         `json:"root_index"`
	LocalIndex  LocalIndexDoc        `json:"local_index"`
	GlobalIndex RootIndexDoc         `json:"global_index"`
}

// IndexEntry is the only payload an index may carry: memory/revision
// references, scope, type, canonical key, derived summary and a page path.
// It never contains prompts, commands, reasoning, credentials or absolute
// paths.
type IndexEntry struct {
	Scope         Scope      `json:"scope"`
	MemoryID      string     `json:"memory_id"`
	MemoryType    MemoryType `json:"memory_type"`
	CanonicalKey  string     `json:"canonical_key"`
	Revision      int        `json:"revision"`
	ContentSHA256 string     `json:"content_sha256"`
	Lifecycle     Lifecycle  `json:"lifecycle"`
	Health        Health     `json:"health"`
	Freshness     Freshness  `json:"freshness"`
	Pinned        bool       `json:"pinned"`
	PagePath      string     `json:"page_path"`
}

// RootIndexDoc is the project (or global) root index: the normal readable
// entries plus separate counts for frozen and archived memories, which do
// not participate in normal retrieval.
type RootIndexDoc struct {
	SchemaVersion int          `json:"schema_version"`
	Scope         string       `json:"scope"`
	Entries       []IndexEntry `json:"entries"`
	FrozenCount   int          `json:"frozen_count"`
	ArchivedCount int          `json:"archived_count"`
}

// LocalIndexDoc is the deterministically sharded local index. Shards follow
// the versioned Index Policy split order; entries that cannot be classified
// or overflow the per-page bound land in the Overflow bucket.
type LocalIndexDoc struct {
	SchemaVersion int                     `json:"schema_version"`
	Scope         string                  `json:"scope"`
	Shards        map[string][]IndexEntry `json:"shards"`
	Overflow      []IndexEntry            `json:"overflow"`
}

// Default freshness window (days) used when no Freshness Policy is supplied.
const (
	defaultEvaluationWindowDays = 90
	defaultAgingAfterDays       = 180
	defaultStaleAfterDays       = 365
)

// defaultIndexPolicy mirrors the MEM-01C example topology but keeps only the
// dimensions a memory fact can actually carry; component/operation are not
// memory dimensions and would always land in the overflow bucket.
var defaultIndexPolicy = PolicyConfigIndex{
	MaxEntriesPerPage: 64,
	MaxPageBytes:      32768,
	MaxShardDepth:     4,
	SplitOrder:        []string{"memory_type", "stable_id_prefix"},
	OverflowBucket:    "other",
	Version:           1,
}

// ---- derivation ----

// DeriveState deterministically derives Lifecycle/Health/Freshness, pinned
// and archived flags, usage statistics and the Root/Local/Global indexes for
// one scope, reading only fully-verified facts through the FactStore. It
// never writes anything: derived state is fully reconstructible and can be
// deleted and rebuilt from the canonical facts alone. Any orphan reference,
// corrupt fact or scope crossing fails closed with a redacted error.
func DeriveState(ctx context.Context, store *FactStore, req DerivedStateRequest) (*DerivedStateResult, error) {
	if err := req.Scope.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid derived state scope")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "derived state scope does not match the store scope")
	}
	in, err := loadDerivedInputs(ctx, store)
	if err != nil {
		return nil, err
	}
	if err := validateDerivedReferences(in); err != nil {
		return nil, err
	}

	fresh := defaultFreshnessPolicy
	if req.FreshnessPolicy != nil {
		fresh = *req.FreshnessPolicy
	}
	idx := defaultIndexPolicy
	if req.IndexPolicy != nil {
		idx = *req.IndexPolicy
	}
	now := req.Now
	if now.IsZero() {
		// Deterministic callers always pass Now; a zero time means "the
		// beginning of time", so everything is fresh — never the wall clock.
		now = time.Unix(0, 0).UTC()
	}

	var states []DerivedMemoryState
	for _, revs := range in.revisions {
		cur := revs[len(revs)-1] // revisions sorted ascending by LoadDerivedInputs
		if req.Revision > 0 {
			r := revisionAt(revs, req.Revision)
			if r == nil {
				return nil, storeError(CodeDerivedInvalidInput, "requested revision does not exist")
			}
			cur = *r
		}
		st := deriveOne(ctx, store, in, cur, fresh, now)
		states = append(states, st)
	}
	sort.Slice(states, func(i, j int) bool { return derivedLess(states[i], states[j]) })

	root, local, global, err := buildIndexes(req.Scope, states, idx)
	if err != nil {
		return nil, err
	}
	return &DerivedStateResult{States: states, RootIndex: root, LocalIndex: local, GlobalIndex: global}, nil
}

// deriveSelectedStates derives only the explicitly selected revisions while
// still validating the complete fact world that can influence them. The
// caller is responsible for recording every returned input in its Generation
// manifest.
func deriveSelectedStates(ctx context.Context, store *FactStore, scope Scope, selected []MemoryRevisionRef, allowed []ManifestInput, now time.Time) ([]DerivedMemoryState, *derivedInputs, error) {
	var in *derivedInputs
	var err error
	if len(allowed) > 0 {
		in, err = loadDerivedInputsFromManifest(ctx, store, allowed)
	} else {
		in, err = loadDerivedInputs(ctx, store)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := validateDerivedReferences(in); err != nil {
		return nil, nil, err
	}
	if now.IsZero() {
		now = time.Unix(0, 0).UTC()
	}
	states := make([]DerivedMemoryState, 0, len(selected))
	for _, ref := range selected {
		rev := revisionAt(in.revisions[ref.MemoryID], ref.Revision)
		if rev == nil || rev.Scope != scope || rev.ContentSHA256 != ref.ContentSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "selected revision is not available for derivation")
		}
		states = append(states, deriveOne(ctx, store, in, *rev, defaultFreshnessPolicy, now))
	}
	sort.Slice(states, func(i, j int) bool { return derivedLess(states[i], states[j]) })
	return states, in, nil
}

func loadDerivedInputsFromManifest(ctx context.Context, store *FactStore, inputs []ManifestInput) (*derivedInputs, error) {
	in := &derivedInputs{revisions: map[string][]MemoryRevision{}, evidence: map[string][]MemoryEvidenceGeneration{}}
	for _, input := range inputs {
		fact, err := readManifestInput(ctx, store, input)
		if err != nil {
			return nil, err
		}
		switch v := fact.(type) {
		case MemoryRevision:
			in.revisions[v.MemoryID] = append(in.revisions[v.MemoryID], v)
		case MemoryEvidenceGeneration:
			key := v.MemoryID + "/" + itoa(v.Revision)
			in.evidence[key] = append(in.evidence[key], v)
		case JudgmentFact:
			in.judgments = append(in.judgments, v)
		case GovernanceEvent:
			in.governance = append(in.governance, v)
		case MemoryUsage:
			in.usages = append(in.usages, v)
		case Outcome:
			in.outcomes = append(in.outcomes, v)
		}
	}
	for id := range in.revisions {
		sort.Slice(in.revisions[id], func(i, j int) bool { return in.revisions[id][i].Revision < in.revisions[id][j].Revision })
	}
	for key := range in.evidence {
		sort.Slice(in.evidence[key], func(i, j int) bool {
			return in.evidence[key][i].EvidenceGeneration < in.evidence[key][j].EvidenceGeneration
		})
	}
	return in, nil
}

// defaultFreshnessPolicy is the frozen fallback when no policy is supplied.
var defaultFreshnessPolicy = PolicyConfigFreshness{
	EvaluationWindowDays:      defaultEvaluationWindowDays,
	AgingAfterDays:            defaultAgingAfterDays,
	StaleAfterDays:            defaultStaleAfterDays,
	RevalidationEvidenceTypes: []string{"test_result"},
	Version:                   1,
}

func revisionAt(revs []MemoryRevision, rev int) *MemoryRevision {
	for i := range revs {
		if revs[i].Revision == rev {
			return &revs[i]
		}
	}
	return nil
}

// derivedInputs holds every fact class the derivation reduces over.
type derivedInputs struct {
	revisions  map[string][]MemoryRevision           // memory_id -> ascending by revision
	evidence   map[string][]MemoryEvidenceGeneration // "mem/rev" -> ascending generation
	judgments  []JudgmentFact
	governance []GovernanceEvent
	usages     []MemoryUsage
	outcomes   []Outcome
}

func loadDerivedInputs(ctx context.Context, store *FactStore) (*derivedInputs, error) {
	in := &derivedInputs{
		revisions: map[string][]MemoryRevision{},
		evidence:  map[string][]MemoryEvidenceGeneration{},
	}
	revKeys, err := store.List(ctx, FactKindMemoryRevision)
	if err != nil {
		return nil, err
	}
	for _, k := range revKeys {
		data, err := store.Get(ctx, FactKindMemoryRevision, k)
		if err != nil {
			return nil, err
		}
		rev, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		in.revisions[rev.MemoryID] = append(in.revisions[rev.MemoryID], rev)
	}
	for id := range in.revisions {
		sort.Slice(in.revisions[id], func(i, j int) bool {
			return in.revisions[id][i].Revision < in.revisions[id][j].Revision
		})
	}
	evKeys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return nil, err
	}
	for _, k := range evKeys {
		data, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, k)
		if err != nil {
			return nil, err
		}
		ev, err := DecodeStrict[MemoryEvidenceGeneration](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		in.evidence[ev.MemoryID+"/"+itoa(ev.Revision)] = append(in.evidence[ev.MemoryID+"/"+itoa(ev.Revision)], ev)
	}
	for key := range in.evidence {
		sort.Slice(in.evidence[key], func(i, j int) bool {
			return in.evidence[key][i].EvidenceGeneration < in.evidence[key][j].EvidenceGeneration
		})
	}
	if in.judgments, err = readAllFacts[JudgmentFact](ctx, store, FactKindJudgment); err != nil {
		return nil, err
	}
	if in.governance, err = readAllFacts[GovernanceEvent](ctx, store, FactKindGovernanceEvent); err != nil {
		return nil, err
	}
	if in.usages, err = readAllFacts[MemoryUsage](ctx, store, FactKindMemoryUsage); err != nil {
		return nil, err
	}
	if in.outcomes, err = readAllFacts[Outcome](ctx, store, FactKindOutcome); err != nil {
		return nil, err
	}
	return in, nil
}

func readAllFacts[T Fact](ctx context.Context, store *FactStore, kind FactKind) ([]T, error) {
	keys, err := store.List(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		data, err := store.Get(ctx, kind, k)
		if err != nil {
			return nil, err
		}
		v, err := DecodeStrict[T](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		out = append(out, v)
	}
	return out, nil
}

// validateDerivedReferences fails closed on any fact that references a
// memory, revision, usage or outcome that does not exist in the same scope:
// an unexplainable record must never silently influence derived state.
func validateDerivedReferences(in *derivedInputs) error {
	revExists := func(scope Scope, id string, rev int) bool {
		revs, ok := in.revisions[id]
		if !ok {
			return false
		}
		for _, r := range revs {
			if r.Revision == rev && r.Scope == scope {
				return true
			}
		}
		return false
	}
	usageIDs := map[string]bool{}
	usageByID := map[string]MemoryUsage{}
	for _, u := range in.usages {
		usageIDs[u.UsageID] = true
		usageByID[u.UsageID] = u
		if !revExists(u.Scope, u.MemoryID, u.Revision) {
			return storeError(CodeDerivedInvalidInput, "usage references a memory revision that does not exist")
		}
	}
	outcomeIDs := map[string]bool{}
	for _, o := range in.outcomes {
		outcomeIDs[o.OutcomeID] = true
		if !revExists(o.Scope, o.MemoryID, o.Revision) {
			return storeError(CodeDerivedInvalidInput, "outcome references a memory revision that does not exist")
		}
		u, ok := usageByID[o.UsageID]
		if !ok {
			return storeError(CodeDerivedInvalidInput, "outcome references a usage that does not exist")
		}
		if u.MemoryID != o.MemoryID || u.Revision != o.Revision {
			return storeError(CodeDerivedInvalidInput, "outcome usage identity does not match")
		}
	}
	for _, g := range in.governance {
		if !revExists(g.Scope, g.MemoryID, g.Revision) {
			return storeError(CodeDerivedInvalidInput, "governance event references a memory revision that does not exist")
		}
	}
	for _, j := range in.judgments {
		switch j.Subject.SubjectType {
		case "memory_revision":
			if j.Subject.MemoryRef == nil {
				return storeError(CodeDerivedInvalidInput, "judgment subject is not readable")
			}
			if !revExists(j.Subject.MemoryRef.Scope, j.Subject.MemoryRef.MemoryID, j.Subject.MemoryRef.Revision) {
				return storeError(CodeDerivedInvalidInput, "judgment references a memory revision that does not exist")
			}
		case "memory_outcome":
			if !outcomeIDs[j.Subject.OutcomeID] {
				return storeError(CodeDerivedInvalidInput, "attribution judgment references an outcome that does not exist")
			}
		}
	}
	return nil
}

// deriveOne computes the full derived state of one revision.
func deriveOne(ctx context.Context, store *FactStore, in *derivedInputs, rev MemoryRevision, fresh PolicyConfigFreshness, now time.Time) DerivedMemoryState {
	gov := applyGovernance(in.governance, rev)
	usage, helpEpisodes, usageByID := deriveUsage(in, rev)
	st := DerivedMemoryState{
		Scope:         rev.Scope,
		MemoryID:      rev.MemoryID,
		MemoryType:    rev.MemoryType,
		CanonicalKey:  rev.CanonicalKey,
		Revision:      rev.Revision,
		ContentSHA256: rev.ContentSHA256,
		UsagePolicy:   rev.UsagePolicy,
		Pinned:        gov.pinned,
		Frozen:        gov.frozen,
		Archived:      gov.archived,
		Usage:         usage,
	}
	st.Lifecycle = deriveLifecycle(in, rev, usage, helpEpisodes, gov)
	// Health is independent of Lifecycle: probation/active/frozen are not
	// automatically degraded; only verified negative evidence (an attributed
	// harmed outcome from an affected/evaluated usage that was never
	// external) degrades health.
	st.Health = deriveHealth(st.Lifecycle, hasHarmedOutcome(in, rev, usageByID))
	st.Freshness = deriveFreshness(in, rev, fresh, now)
	return st
}

// governanceState is the applied governance intent chain for one memory.
type governanceState struct {
	pinned   bool
	frozen   bool
	archived bool
}

// applyGovernance applies the revision's governance events in deterministic
// time order. The event's revision is part of its immutable target identity:
// a new revision never inherits pin/freeze/archive intent from an older one.
// Archived is terminal for that revision: later intent is ignored.
func applyGovernance(events []GovernanceEvent, rev MemoryRevision) governanceState {
	var mine []GovernanceEvent
	for _, g := range events {
		if g.MemoryID == rev.MemoryID && g.Revision == rev.Revision {
			mine = append(mine, g)
		}
	}
	sort.Slice(mine, func(i, j int) bool {
		ti, _ := normalizeTime(mine[i].CreatedAt)
		tj, _ := normalizeTime(mine[j].CreatedAt)
		if ti != tj {
			return ti < tj
		}
		return mine[i].EventID < mine[j].EventID
	})
	var gs governanceState
	for _, g := range mine {
		if gs.archived {
			continue // terminal
		}
		switch g.Operation {
		case "pin":
			gs.pinned = true
		case "unpin":
			gs.pinned = false
		case "manual_freeze":
			gs.frozen = true
		case "unfreeze":
			gs.frozen = false
		case "archive":
			gs.archived = true
		}
	}
	return gs
}

// deriveLifecycle computes the promotion state by Usage Policy, then applies
// the governance intent (archived and frozen win over evidence state). The
// outcome_attributed thresholds follow the frozen scoring protocol: at least
// three counted helps across at least two independent episodes for active,
// the first attributed harm for degraded, and three attributed harms with a
// >= 60% negative rate for auto-frozen. Success never cancels failure and
// insufficient evidence stays probation.
func deriveLifecycle(in *derivedInputs, rev MemoryRevision, usage UsageStats, helpEpisodes int, gov governanceState) Lifecycle {
	if gov.archived {
		return LifecycleArchived
	}
	if !isLatestRevision(in, rev) {
		return LifecycleSuperseded
	}
	if gov.frozen {
		return LifecycleFrozen
	}
	switch rev.UsagePolicy {
	case UsagePolicyOutcomeAttributed:
		pos := usage.CountedHelpCount
		neg := usage.CountedHarmCount
		if neg >= 3 && (pos+neg) > 0 && float64(neg)/float64(pos+neg) >= 0.6 {
			return LifecycleFrozen
		}
		if neg >= 1 {
			return LifecycleDegraded
		}
		if pos >= 3 && helpEpisodes >= 2 {
			return LifecycleActive
		}
		return LifecycleProbation
	case UsagePolicyEvidenceValidated:
		// Architecture 11.5 active conditions: >=3 independent EvidenceRefs,
		// >=2 independent Root Tasks / formal sources (root_task_refs), no
		// unresolved conflicts (no contradicts relations) AND a passing
		// Critic Judgment. The Critic judgment subtype is not registered
		// (architecture 6.2.3 requires a fixed payload protocol that later
		// phases define); per the CTO decision this stays an open protocol
		// gap, so the active condition cannot be satisfied and
		// evidence_validated remains probation until the Critic protocol
		// lands. See the MEM-01F plan "未决协议项".
		return LifecycleProbation
	case UsagePolicyExplicitConfirmation:
		switch confirmationStatus(in, rev) {
		case "confirmed":
			return LifecycleActive
		case "revoked":
			// The confirmation was explicitly revoked and no replacement
			// revision exists (superseded was already handled above):
			// frozen, never a simplified degraded.
			return LifecycleFrozen
		case "unverifiable":
			// Architecture 11.5: a confirmation that is temporarily
			// unverifiable degrades instead of activating.
			return LifecycleDegraded
		default:
			return LifecycleProbation
		}
	default:
		return LifecycleProbation
	}
}

func isLatestRevision(in *derivedInputs, rev MemoryRevision) bool {
	revs := in.revisions[rev.MemoryID]
	if len(revs) == 0 {
		return true
	}
	return revs[len(revs)-1].Revision == rev.Revision
}

// confirmationMatches reports whether the judgment is a confirmation for
// exactly this memory revision: it must be a confirmation, live in the same
// scope, and its subject must be a memory_revision MemoryRef with the same
// scope, memory_id and revision. Any node in the supersede chain that stops
// describing the current revision invalidates the whole chain.
func confirmationMatches(j JudgmentFact, rev MemoryRevision) bool {
	if j.JudgmentType != JudgmentTypeConfirmation || j.Scope != rev.Scope {
		return false
	}
	if j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil {
		return false
	}
	r := j.Subject.MemoryRef
	return r.Scope == rev.Scope && r.MemoryID == rev.MemoryID && r.Revision == rev.Revision
}

// confirmationStatus resolves the effective confirmation of an
// explicit_confirmation revision per the architecture 11.5 table. The
// revision's confirmation_source_ref is the anchor: the referenced judgment
// must exist and must be a confirmation describing exactly this memory
// revision (else "unverifiable" -> degraded, never active/frozen). A
// supersede chain from it is followed to the top; each superseding node must
// pass the same match, otherwise the chain is unverifiable. Returns
// "confirmed", "revoked", "unverifiable" or "".
func confirmationStatus(in *derivedInputs, rev MemoryRevision) string {
	if rev.ConfirmationSourceRef == nil {
		return ""
	}
	var cur *JudgmentFact
	for i := range in.judgments {
		if in.judgments[i].JudgmentID == rev.ConfirmationSourceRef.JudgmentID {
			cur = &in.judgments[i]
			break
		}
	}
	if cur == nil {
		return "unverifiable"
	}
	if !confirmationMatches(*cur, rev) {
		return "unverifiable"
	}
	// Follow the supersede chain to the top: a revocation that supersedes
	// the referenced confirmation replaces its status. Every superseding
	// node must still describe exactly this memory revision; a foreign node
	// invalidates the chain. When several judgments supersede the same node
	// (the schema does not enforce uniqueness), the newest one wins, matching
	// latestFreshnessJudgment / attribution-override semantics. A visited set
	// bounds the walk so a self- or mutual supersede cycle fails closed
	// instead of looping forever.
	visited := map[string]bool{cur.JudgmentID: true}
	for {
		var next *JudgmentFact
		for i := range in.judgments {
			j := in.judgments[i]
			if j.SupersedesJudgmentRef == nil || j.SupersedesJudgmentRef.JudgmentID != cur.JudgmentID {
				continue
			}
			if !confirmationMatches(j, rev) {
				return "unverifiable"
			}
			if visited[j.JudgmentID] {
				return "unverifiable" // supersede cycle: conservative fail-closed
			}
			if next == nil || judgmentNewer(j, *next) {
				next = &in.judgments[i]
			}
		}
		if next == nil {
			break
		}
		visited[next.JudgmentID] = true
		cur = next
	}
	if cur.Confirmation != nil {
		return cur.Confirmation.Status
	}
	return ""
}

// judgmentNewer orders two judgments by created_at with judgment_id as the
// deterministic tie-break (matching the other latest-wins resolutions).
func judgmentNewer(a, b JudgmentFact) bool {
	ta, _ := normalizeTime(a.CreatedAt)
	tb, _ := normalizeTime(b.CreatedAt)
	if ta != tb {
		return ta > tb
	}
	return a.JudgmentID > b.JudgmentID
}

// deriveHealth is the independent health dimension. A lifecycle name alone
// never degrades health: probation/active/frozen/superseded/archived are
// healthy unless verified negative evidence exists; only the degraded
// lifecycle or a harmed outcome degrades health. This supports every
// independent combination (active+healthy, active+degraded, frozen+healthy,
// frozen+degraded).
func deriveHealth(lc Lifecycle, hasNegative bool) Health {
	switch lc {
	case LifecycleActive, LifecycleProbation, LifecycleSuperseded, LifecycleArchived:
		if hasNegative {
			return HealthDegraded
		}
		return HealthHealthy
	case LifecycleDegraded:
		return HealthDegraded
	case LifecycleFrozen:
		if hasNegative {
			return HealthDegraded
		}
		return HealthHealthy
	default:
		return HealthDegraded
	}
}

// hasHarmedOutcome reports verified negative evidence for the revision: an
// outcome whose raw effect was harmed, that belongs to an affected/evaluated
// usage (the same stage filter as attribution) and was not an external
// failure. Overrides may re-classify an outcome (changing counted help/harm),
// but the historical harm stays as negative evidence — success never cancels
// failure. Earlier-stage or external harms are not verified negative
// evidence and never degrade health.
func hasHarmedOutcome(in *derivedInputs, rev MemoryRevision, usageByID map[string]MemoryUsage) bool {
	for _, o := range in.outcomes {
		if o.MemoryID != rev.MemoryID || o.Revision != rev.Revision {
			continue
		}
		if o.ExternalFailure {
			continue
		}
		if o.enriched() && (o.CountedAsHarm == nil || !*o.CountedAsHarm) {
			continue
		}
		if o.Effect != "harmed" {
			continue
		}
		u, ok := usageByID[o.UsageID]
		if !ok || !usageStageAttributed(u.UsageStage) {
			continue
		}
		return true
	}
	return false
}

// deriveFreshness prefers the newest non-superseded freshness_evaluation
// judgment; without one it falls back to the Freshness Policy window against
// the latest activity time. It can only ever return fresh/aging/
// needs_revalidation.
func deriveFreshness(in *derivedInputs, rev MemoryRevision, fresh PolicyConfigFreshness, now time.Time) Freshness {
	judgment := latestFreshnessJudgment(in, rev)
	if judgment != nil && judgment.FreshnessEvaluation != nil {
		return Freshness(judgment.FreshnessEvaluation.Result)
	}
	last := latestActivity(in, rev)
	if last.IsZero() {
		last = parseFactTime(rev.CreatedAt)
	}
	ageDays := int(now.Sub(last).Hours() / 24)
	if ageDays < fresh.EvaluationWindowDays {
		return FreshnessFresh
	}
	if ageDays < fresh.AgingAfterDays {
		return FreshnessAging
	}
	return FreshnessNeedsRevalidation
}

func latestFreshnessJudgment(in *derivedInputs, rev MemoryRevision) *JudgmentFact {
	superBy := map[string]bool{}
	for _, j := range in.judgments {
		if j.SupersedesJudgmentRef != nil {
			superBy[j.SupersedesJudgmentRef.JudgmentID] = true
		}
	}
	var candidates []JudgmentFact
	for i := range in.judgments {
		j := in.judgments[i]
		if j.JudgmentType != JudgmentTypeFreshnessEvaluation || superBy[j.JudgmentID] {
			continue
		}
		if j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil {
			continue
		}
		if j.Subject.MemoryRef.MemoryID == rev.MemoryID && j.Subject.MemoryRef.Revision == rev.Revision {
			candidates = append(candidates, j)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		ti, _ := normalizeTime(candidates[i].CreatedAt)
		tj, _ := normalizeTime(candidates[j].CreatedAt)
		if ti != tj {
			return ti < tj
		}
		return candidates[i].JudgmentID < candidates[j].JudgmentID
	})
	return &candidates[len(candidates)-1]
}

// latestActivity is the newest timestamp across usages, evidence and the
// revision itself (deterministic; missing facts are skipped).
func latestActivity(in *derivedInputs, rev MemoryRevision) time.Time {
	latest := time.Time{}
	consider := func(s string) {
		t := parseFactTime(s)
		if !t.IsZero() && t.After(latest) {
			latest = t
		}
	}
	for _, u := range in.usages {
		if u.MemoryID == rev.MemoryID && u.Revision == rev.Revision {
			consider(u.OccurredAt)
		}
	}
	for _, evs := range in.evidence[rev.MemoryID+"/"+itoa(rev.Revision)] {
		consider(evs.CreatedAt)
	}
	return latest
}

// deriveUsage aggregates the usage events and their effective outcomes for
// one revision. Repeats are impossible (the FactStore rejects duplicate
// usage_id), and the aggregation itself is keyed by usage_id, so a repeated
// event can never double count. Only affected/evaluated usages produce
// attribution evidence (help/harm); earlier stages count as usage and as
// recency but never attribute. The second return value is the number of
// independent episodes behind counted helps, required by the active
// threshold (usage_id is never used as an episode stand-in).
func deriveUsage(in *derivedInputs, rev MemoryRevision) (UsageStats, int, map[string]MemoryUsage) {
	var stats UsageStats
	seen := map[string]bool{}
	usageByID := map[string]MemoryUsage{}
	effective := map[string]string{} // outcome_id -> effective effect
	latest := time.Time{}
	for _, u := range in.usages {
		if u.MemoryID != rev.MemoryID || u.Revision != rev.Revision || seen[u.UsageID] {
			continue
		}
		seen[u.UsageID] = true
		usageByID[u.UsageID] = u
		stats.UsageCount++
		if t := parseFactTime(u.OccurredAt); t.After(latest) {
			latest = t
		}
	}
	for _, o := range in.outcomes {
		if o.MemoryID != rev.MemoryID || o.Revision != rev.Revision {
			continue
		}
		effective[o.OutcomeID] = o.Effect
	}
	// The newest non-superseded attribution override per outcome wins,
	// mirroring confirmationStatus / latestFreshnessJudgment: superseded
	// overrides are excluded by judgment id, then created_at (id as tie
	// break) selects the newest remaining override.
	superBy := map[string]bool{}
	for _, j := range in.judgments {
		if j.SupersedesJudgmentRef != nil {
			superBy[j.SupersedesJudgmentRef.JudgmentID] = true
		}
	}
	byOutcome := map[string][]JudgmentFact{}
	for _, j := range in.judgments {
		if j.JudgmentType != JudgmentTypeAttributionOverride || j.Subject.SubjectType != "memory_outcome" {
			continue
		}
		if superBy[j.JudgmentID] {
			continue
		}
		if _, ok := effective[j.Subject.OutcomeID]; !ok {
			continue
		}
		byOutcome[j.Subject.OutcomeID] = append(byOutcome[j.Subject.OutcomeID], j)
	}
	for id, list := range byOutcome {
		sort.Slice(list, func(i, j int) bool {
			ti, _ := normalizeTime(list[i].CreatedAt)
			tj, _ := normalizeTime(list[j].CreatedAt)
			if ti != tj {
				return ti < tj
			}
			return list[i].JudgmentID < list[j].JudgmentID
		})
		latestOverride := list[len(list)-1]
		if latestOverride.AttributionOverride != nil {
			effective[id] = latestOverride.AttributionOverride.NewEffect
		}
	}
	attributed := false
	helpEpisodes := map[string]bool{}
	for _, o := range in.outcomes {
		if o.MemoryID != rev.MemoryID || o.Revision != rev.Revision {
			continue
		}
		if !seen[o.UsageID] {
			continue
		}
		u := usageByID[o.UsageID]
		if !usageStageAttributed(u.UsageStage) {
			continue // retrieved/read/adopted never attribute
		}
		if o.ExternalFailure {
			continue // third-party failure is never auto-attributed
		}
		effect := effective[o.OutcomeID]
		if o.enriched() {
			if o.Evaluated == nil || !*o.Evaluated || o.Attribution != "confirmed" {
				continue
			}
			if effect == "harmed" && o.Critic != "supported" {
				continue
			}
		}
		attributed = true
		switch effect {
		case "helped":
			stats.CountedHelpCount++
			if u.EpisodeID != "" {
				helpEpisodes[u.EpisodeID] = true
			}
		case "harmed":
			stats.CountedHarmCount++
		}
	}
	if !latest.IsZero() {
		stats.LastUsedAt = latest.UTC().Format(time.RFC3339)
	}
	// A usage with no attributable (non-external) outcome is insufficient
	// evidence, never a guessed effect.
	stats.InsufficientEvidence = stats.UsageCount > 0 && !attributed
	return stats, len(helpEpisodes), usageByID
}

func parseFactTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Also accept the canonical RFC3339Nano form produced by normalizeTime.
	t, err = time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ---- stable ordering ----

var lifecycleRank = map[Lifecycle]int{
	LifecycleActive:     0,
	LifecycleProbation:  1,
	LifecycleDegraded:   2,
	LifecycleFrozen:     3,
	LifecycleSuperseded: 4,
	LifecycleArchived:   5,
}

var healthRank = map[Health]int{
	HealthHealthy:  0,
	HealthDegraded: 1,
}

// derivedLess implements the frozen ordering: lifecycle/health first (pinned
// lifts only within an evidence class, never across lifecycle), then
// evidence strength, successful reuse count, recency, stable id and finally
// a reproducible deterministic seed. No wall clock or global random source
// is ever consulted.
func derivedLess(a, b DerivedMemoryState) bool {
	if r := lifecycleRank[a.Lifecycle] - lifecycleRank[b.Lifecycle]; r != 0 {
		return r < 0
	}
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	if r := healthRank[a.Health] - healthRank[b.Health]; r != 0 {
		return r < 0
	}
	if s := evidenceStrength(a) - evidenceStrength(b); s != 0 {
		return s > 0
	}
	if a.Usage.CountedHelpCount != b.Usage.CountedHelpCount {
		return a.Usage.CountedHelpCount > b.Usage.CountedHelpCount
	}
	if a.Usage.LastUsedAt != b.Usage.LastUsedAt {
		return a.Usage.LastUsedAt > b.Usage.LastUsedAt // most recent first
	}
	if a.MemoryID != b.MemoryID {
		return a.MemoryID < b.MemoryID
	}
	return hashOf([]byte(a.CanonicalKey+a.MemoryID)) < hashOf([]byte(b.CanonicalKey+b.MemoryID))
}

// evidenceStrength is the usage-policy-specific evidence quantity used by
// the ordering: counted outcomes for outcome_attributed, evidence references
// for evidence_validated, and 1 when confirmed for explicit_confirmation.
func evidenceStrength(st DerivedMemoryState) int {
	switch st.UsagePolicy {
	case UsagePolicyOutcomeAttributed:
		return st.Usage.CountedHelpCount + st.Usage.CountedHarmCount
	case UsagePolicyEvidenceValidated:
		// Evidence strength is carried by the lifecycle rank; the ordering
		// reuses the success-reuse count as the generic quantity.
		return st.Usage.CountedHelpCount
	case UsagePolicyExplicitConfirmation:
		if st.Lifecycle == LifecycleActive {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// ---- indexes ----

func buildIndexes(scope Scope, states []DerivedMemoryState, idx PolicyConfigIndex) (RootIndexDoc, LocalIndexDoc, RootIndexDoc, error) {
	root := RootIndexDoc{SchemaVersion: SchemaVersion, Scope: string(scope)}
	local := LocalIndexDoc{SchemaVersion: SchemaVersion, Scope: string(scope), Shards: map[string][]IndexEntry{}}
	global := RootIndexDoc{}
	if scope == ScopeGlobal {
		global = RootIndexDoc{SchemaVersion: SchemaVersion, Scope: string(scope)}
	}
	for _, st := range states {
		entry, ok := indexEntry(st)
		if !ok {
			continue
		}
		switch st.Lifecycle {
		case LifecycleFrozen:
			root.FrozenCount++
			continue
		case LifecycleArchived:
			root.ArchivedCount++
			continue
		}
		root.Entries = append(root.Entries, entry)
		if scope == ScopeGlobal {
			global.Entries = append(global.Entries, entry)
		}
	}
	tree, err := CompileIndexTree(scope, states, idx)
	if err != nil {
		return RootIndexDoc{}, LocalIndexDoc{}, RootIndexDoc{}, err
	}
	projectIndexLeaves(tree.Root, local.Shards)
	return root, local, global, nil
}

func projectIndexLeaves(node *IndexNode, out map[string][]IndexEntry) {
	if node == nil {
		return
	}
	if len(node.Entries) > 0 {
		out[node.Path] = append([]IndexEntry{}, node.Entries...)
	}
	for _, child := range node.children {
		projectIndexLeaves(child, out)
	}
}

// indexEntry builds an index entry; a page path that cannot be rendered
// safely is not an index failure, the entry is skipped (canonical keys are
// already validated by the schema, so this is defensive).
func indexEntry(st DerivedMemoryState) (IndexEntry, bool) {
	dir := okfTypeDir(st.MemoryType)
	if dir == "unknown" {
		return IndexEntry{}, false
	}
	return IndexEntry{
		Scope:         st.Scope,
		MemoryID:      st.MemoryID,
		MemoryType:    st.MemoryType,
		CanonicalKey:  st.CanonicalKey,
		Revision:      st.Revision,
		ContentSHA256: st.ContentSHA256,
		Lifecycle:     st.Lifecycle,
		Health:        st.Health,
		Freshness:     st.Freshness,
		Pinned:        st.Pinned,
		PagePath:      "wiki/" + dir + "/" + st.CanonicalKey + ".md",
	}, true
}

func indexEntryLess(a, b IndexEntry) bool {
	return derivedLess(stateFromEntry(a), stateFromEntry(b))
}

func stateFromEntry(e IndexEntry) DerivedMemoryState {
	return DerivedMemoryState{
		MemoryID: e.MemoryID, Lifecycle: e.Lifecycle, Health: e.Health,
		Usage: UsageStats{CountedHelpCount: 0}, CanonicalKey: e.CanonicalKey,
		Pinned: e.Pinned,
	}
}

// localShardKey renders the shard path from the index policy split order.
// Dimensions a memory fact cannot carry (component, operation) are skipped;
// if nothing classifiable remains, the entry goes to the overflow bucket.
func localShardKey(e IndexEntry, idx PolicyConfigIndex) string {
	var parts []string
	for _, d := range idx.SplitOrder {
		switch d {
		case "memory_type":
			parts = append(parts, "memory_type/"+string(e.MemoryType))
		case "stable_id_prefix":
			parts = append(parts, "stable_id_prefix/"+stableIDPrefix(e.MemoryID))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > idx.MaxShardDepth {
		return ""
	}
	return strings.Join(parts, "/")
}

// stableIDPrefix derives a stable one-character shard prefix from the memory
// id after the "mem_" scheme prefix, so shards stay small and stable.
func stableIDPrefix(id string) string {
	s := strings.TrimPrefix(id, "mem_")
	if s == "" {
		return "x"
	}
	return s[:1]
}

// ---- errors ----

// IsSensitiveError reports whether err is a redacted StoreError.
func IsSensitiveError(err error) bool {
	var se *StoreError
	return errors.As(err, &se)
}
