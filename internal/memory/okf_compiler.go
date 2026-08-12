package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The OKF compiler (MEM-01E) is a pure, model-free, deterministic compiler:
// explicit MemoryRevision + MemoryEvidenceGeneration references in, and a
// fixed set of OKF Wiki pages, index and derived state views out. Every view
// is derivable byte-for-byte from the same canonical facts and compiler
// version; nothing written here is a second fact source.

// Frozen compiler identity. The registry in generation_tx.go must map this
// exact pair; anything else returns memory_generation_compiler_unavailable.
const (
	OKFCompilerVersionV1             = "mnemosyne-okf-compiler/1"
	OKFCompilerVersion               = "mnemosyne-okf-compiler/2"
	OKFCanonicalizationVersion       = 1
	okfOutputSchemaVersion           = 1
	okfFrontmatterVersion            = "0.1"
	okfNotAvailable                  = "not_available"
	maxCompiledOutputBytes     int64 = 1 << 20 // 1 MiB per view file
)

func compileOKFLegacy(ctx context.Context, store *FactStore, req OKFCompileRequest) (*OKFCompileResult, error) {
	revisions, evidenceByKey, err := loadOKFInputs(ctx, store, req)
	if err != nil {
		return nil, err
	}
	res := &OKFCompileResult{Outputs: map[string][]byte{}}
	if err := compileOKFLegacyViews(res, req.Scope, revisions, evidenceByKey); err != nil {
		return nil, err
	}
	res.CompiledSHA256 = compiledOutputHash(res.Outputs)
	return res, nil
}

// MemoryRevisionRef explicitly references one canonical MemoryRevision.
type MemoryRevisionRef struct {
	MemoryID      string
	Revision      int
	ContentSHA256 string
}

// MemoryEvidenceRef explicitly references one canonical
// MemoryEvidenceGeneration.
type MemoryEvidenceRef struct {
	MemoryID           string
	Revision           int
	EvidenceGeneration int
	EvidenceSetSHA256  string
}

// OKFCompileRequest declares the exact input set of one OKF compilation.
// The compiler never scans directories or guesses facts.
type OKFCompileRequest struct {
	Scope            Scope
	BaseGeneration   *string
	IndexPolicyRef   PolicyRef
	EvaluationTime   time.Time
	DerivationInputs []ManifestInput
	Revisions        []MemoryRevisionRef
	Evidence         []MemoryEvidenceRef
}

// OKFCompileResult is the deterministic output of one compilation.
type OKFCompileResult struct {
	// Outputs maps a generation-relative path ("wiki/strategies/x.md",
	// "state/memories.json", ...) to the exact UTF-8 bytes.
	Outputs map[string][]byte
	// CompiledSHA256 is the deterministic hash of the whole view set; it is
	// empty only when the view set is empty.
	CompiledSHA256 string
	// Inputs are the Manifest inputs for the compiled fact set, derived from
	// the exact stored facts (never from user-supplied hashes).
	Inputs []ManifestInput
}

// CompileOKF loads the explicitly referenced facts through the full
// FactStore verification chain and compiles the OKF views. It never writes
// anything; the caller stages the outputs through GenerationStore.
func CompileOKF(ctx context.Context, store *FactStore, req OKFCompileRequest) (*OKFCompileResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "compile cancelled")
	}
	if err := req.Scope.Validate(); err != nil {
		return nil, storeError(CodeOKFInvalidInput, "invalid compile scope")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "compile scope does not match store scope")
	}
	if req.BaseGeneration != nil {
		if err := validateID(*req.BaseGeneration, "base_generation"); err != nil {
			return nil, storeError(CodeOKFInvalidInput, "invalid base generation")
		}
	}
	if err := validateOKFRefs(req); err != nil {
		return nil, err
	}
	if err := req.IndexPolicyRef.Validate(); err != nil || req.IndexPolicyRef.PolicyType != PolicyTypeIndex {
		return nil, storeError(CodeOKFInvalidInput, "invalid index policy reference")
	}
	indexPolicy, err := NewPolicyStore(store).GetPolicy(ctx, req.IndexPolicyRef)
	if err != nil {
		return nil, err
	}

	revisions, evidenceByKey, err := loadOKFInputs(ctx, store, req)
	if err != nil {
		return nil, err
	}

	states, derivedInputs, err := deriveSelectedStates(ctx, store, req.Scope, req.Revisions, req.DerivationInputs, req.EvaluationTime)
	if err != nil {
		return nil, err
	}
	res := &OKFCompileResult{Outputs: map[string][]byte{}}
	if err := compileOKFViews(res, req.Scope, revisions, evidenceByKey, states, *indexPolicy.Config.Index, req.IndexPolicyRef); err != nil {
		return nil, err
	}
	res.CompiledSHA256 = compiledOutputHash(res.Outputs)
	inputs, err := okfManifestInputs(revisions, evidenceByKey, indexPolicy, derivedInputs)
	if err != nil {
		return nil, storeError(CodeOKFCompileError, "cannot derive manifest inputs")
	}
	res.Inputs = inputs
	return res, nil
}

// validateOKFRefs rejects malformed and duplicate explicit references.
func validateOKFRefs(req OKFCompileRequest) error {
	seenRev := map[string]bool{}
	for _, r := range req.Revisions {
		if err := validateID(r.MemoryID, "memory_id"); err != nil {
			return storeError(CodeOKFInvalidInput, "invalid memory revision reference")
		}
		if r.Revision < 1 {
			return storeError(CodeOKFInvalidInput, "invalid memory revision reference")
		}
		if err := validateHash(r.ContentSHA256, "content_sha256"); err != nil {
			return storeError(CodeOKFInvalidInput, "invalid memory revision reference")
		}
		key := r.MemoryID + "\x00" + itoa(r.Revision)
		if seenRev[key] {
			return storeError(CodeOKFInvalidInput, "duplicate memory revision reference")
		}
		seenRev[key] = true
	}
	seenEv := map[string]bool{}
	perRevision := map[string]int{}
	for _, e := range req.Evidence {
		if err := validateID(e.MemoryID, "memory_id"); err != nil {
			return storeError(CodeOKFInvalidInput, "invalid evidence reference")
		}
		if e.Revision < 1 || e.EvidenceGeneration < 1 {
			return storeError(CodeOKFInvalidInput, "invalid evidence reference")
		}
		if err := validateHash(e.EvidenceSetSHA256, "evidence_set_sha256"); err != nil {
			return storeError(CodeOKFInvalidInput, "invalid evidence reference")
		}
		key := e.MemoryID + "\x00" + itoa(e.Revision) + "\x00" + itoa(e.EvidenceGeneration)
		if seenEv[key] {
			return storeError(CodeOKFInvalidInput, "duplicate evidence reference")
		}
		seenEv[key] = true
		perRevision[e.MemoryID+"\x00"+itoa(e.Revision)]++
	}
	for _, n := range perRevision {
		if n != 1 {
			// Each revision takes exactly one evidence generation; zero is
			// handled by the pairing check, more than one fails closed.
			return storeError(CodeOKFInvalidInput, "a revision must reference exactly one evidence generation")
		}
	}
	return nil
}

// loadedRevision is a verified revision plus its verified evidence set.
type loadedRevision struct {
	revision MemoryRevision
	evidence MemoryEvidenceGeneration
}

func loadOKFInputs(ctx context.Context, store *FactStore, req OKFCompileRequest) ([]loadedRevision, map[string]MemoryEvidenceGeneration, error) {
	revs := make([]loadedRevision, 0, len(req.Revisions))
	evidenceByKey := map[string]MemoryEvidenceGeneration{}
	for _, er := range req.Evidence {
		key := er.MemoryID + "\x00" + itoa(er.Revision)
		data, err := store.Get(ctx, FactKindMemoryEvidenceGeneration,
			fmt.Sprintf("%s/%d/%d", er.MemoryID, er.Revision, er.EvidenceGeneration))
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "evidence generation is missing or unreadable")
		}
		ev, err := DecodeStrict[MemoryEvidenceGeneration](data)
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "evidence generation violates the schema")
		}
		if ev.MemoryID != er.MemoryID || ev.Revision != er.Revision || ev.EvidenceGeneration != er.EvidenceGeneration {
			return nil, nil, storeError(CodeOKFInvalidInput, "evidence identity mismatch")
		}
		if ev.EvidenceSetSHA256 != er.EvidenceSetSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "evidence set hash mismatch")
		}
		evidenceByKey[key] = ev
	}
	for _, rr := range req.Revisions {
		data, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", rr.MemoryID, rr.Revision))
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "memory revision is missing or unreadable")
		}
		rev, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "memory revision violates the schema")
		}
		if rev.MemoryID != rr.MemoryID || rev.Revision != rr.Revision {
			return nil, nil, storeError(CodeOKFInvalidInput, "revision identity mismatch")
		}
		if rev.ContentSHA256 != rr.ContentSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "revision content hash mismatch")
		}
		if rev.Scope != req.Scope {
			return nil, nil, storeError(CodeScopeMismatch, "revision scope does not match compile scope")
		}
		// Defense in depth: the canonical key must be path-safe even if a
		// future writer stored something the model validator missed.
		if err := validateCanonicalKey(rev.CanonicalKey); err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "revision canonical key is not path-safe")
		}
		ev, ok := evidenceByKey[rr.MemoryID+"\x00"+itoa(rr.Revision)]
		if !ok {
			return nil, nil, storeError(CodeOKFInvalidInput, "revision has no evidence generation")
		}
		revs = append(revs, loadedRevision{revision: rev, evidence: ev})
	}
	// Orphan evidence references (no revision in this compile) are rejected.
	for _, er := range req.Evidence {
		found := false
		for _, rr := range req.Revisions {
			if rr.MemoryID == er.MemoryID && rr.Revision == er.Revision {
				found = true
				break
			}
		}
		if !found {
			return nil, nil, storeError(CodeOKFInvalidInput, "evidence reference has no matching revision")
		}
	}
	return revs, evidenceByKey, nil
}

func compileOKFViews(res *OKFCompileResult, scope Scope, revisions []loadedRevision, evidenceByKey map[string]MemoryEvidenceGeneration, states []DerivedMemoryState, policy PolicyConfigIndex, policyRef PolicyRef) error {
	sorted := append([]loadedRevision{}, revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].revision, sorted[j].revision
		if a.MemoryType != b.MemoryType {
			return a.MemoryType < b.MemoryType
		}
		if a.CanonicalKey != b.CanonicalKey {
			return a.CanonicalKey < b.CanonicalKey
		}
		if a.MemoryID != b.MemoryID {
			return a.MemoryID < b.MemoryID
		}
		return a.Revision < b.Revision
	})

	pages := make([]okfPage, 0, len(sorted))
	used := map[string]bool{}
	for _, lr := range sorted {
		page, rel, err := compileOKFPage(lr, scope, used)
		if err != nil {
			return err
		}
		res.Outputs[rel] = []byte(page)
		pages = append(pages, okfPage{rel: rel, revision: lr.revision})
	}
	pagePaths := map[string]string{}
	for _, page := range pages {
		pagePaths[page.revision.MemoryID+"\x00"+itoa(page.revision.Revision)] = page.rel
	}
	tree, err := compileIndexTree(scope, states, policy, &policyRef, pagePaths)
	if err != nil {
		return err
	}
	if err := compileIndexOutputs(res.Outputs, tree, policy); err != nil {
		return err
	}
	memories, err := compileOKFMemories(scope, pages, evidenceByKey)
	if err != nil {
		return err
	}
	res.Outputs["state/memories.json"] = memories
	// Relations may only target members of this generation's input set; the
	// identity map keys each input revision by (scope, type, id, revision)
	// so missing targets, wrong revisions, wrong types and wrong content
	// hashes all fail closed instead of emitting dangling links.
	inputs := make(map[string]MemoryRevision, len(revisions))
	for _, lr := range revisions {
		rev := lr.revision
		inputs[relationTargetKey(rev.Scope, rev.MemoryType, rev.MemoryID, rev.Revision)] = rev
	}
	relations, err := compileOKFRelations(scope, pages, inputs)
	if err != nil {
		return err
	}
	res.Outputs["state/relations.json"] = relations
	return nil
}

func compileOKFLegacyViews(res *OKFCompileResult, scope Scope, revisions []loadedRevision, evidenceByKey map[string]MemoryEvidenceGeneration) error {
	sorted := append([]loadedRevision{}, revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].revision, sorted[j].revision
		if a.MemoryType != b.MemoryType {
			return a.MemoryType < b.MemoryType
		}
		if a.CanonicalKey != b.CanonicalKey {
			return a.CanonicalKey < b.CanonicalKey
		}
		if a.MemoryID != b.MemoryID {
			return a.MemoryID < b.MemoryID
		}
		return a.Revision < b.Revision
	})
	pages := make([]okfPage, 0, len(sorted))
	used := map[string]bool{}
	for _, lr := range sorted {
		page, rel, err := compileOKFPage(lr, scope, used)
		if err != nil {
			return err
		}
		res.Outputs[rel] = []byte(page)
		pages = append(pages, okfPage{rel: rel, revision: lr.revision})
	}
	res.Outputs["wiki/index.md"] = []byte(compileOKFIndex(scope, pages))
	memories, err := compileOKFMemories(scope, pages, evidenceByKey)
	if err != nil {
		return err
	}
	res.Outputs["state/memories.json"] = memories
	inputs := make(map[string]MemoryRevision, len(revisions))
	for _, lr := range revisions {
		r := lr.revision
		inputs[relationTargetKey(r.Scope, r.MemoryType, r.MemoryID, r.Revision)] = r
	}
	relations, err := compileOKFRelations(scope, pages, inputs)
	if err != nil {
		return err
	}
	res.Outputs["state/relations.json"] = relations
	return nil
}

// relationTargetKey identifies one input revision as a relation target
// candidate. It is a total key: scope, memory_type, memory_id and revision
// must all match the input revision exactly.
func relationTargetKey(scope Scope, mt MemoryType, id string, rev int) string {
	return string(scope) + "\x00" + string(mt) + "\x00" + id + "\x00" + itoa(rev)
}

// okfPage tracks one compiled page for index/state derivation.
type okfPage struct {
	rel      string
	revision MemoryRevision
}

// okfTypeDir maps a MemoryType onto its OKF wiki directory.
func okfTypeDir(t MemoryType) string {
	switch t {
	case MemoryTypeComponent:
		return "components"
	case MemoryTypeDecision:
		return "decisions"
	case MemoryTypeFailureConcept:
		return "failure-concepts"
	case MemoryTypePattern:
		return "patterns"
	case MemoryTypePlaybook:
		return "playbooks"
	case MemoryTypePreference:
		return "preferences"
	case MemoryTypeStrategy:
		return "strategies"
	default:
		return "unknown"
	}
}

// okfTypeOrder fixes the type listing order in the index.
var okfTypeOrder = []MemoryType{
	MemoryTypeComponent,
	MemoryTypeDecision,
	MemoryTypeFailureConcept,
	MemoryTypePattern,
	MemoryTypePlaybook,
	MemoryTypePreference,
	MemoryTypeStrategy,
}

func compileOKFPage(lr loadedRevision, scope Scope, used map[string]bool) (string, string, error) {
	rev := lr.revision
	if rev.Scope != scope {
		return "", "", storeError(CodeOKFCompileError, "page scope mismatch")
	}
	if err := validateCanonicalKey(rev.CanonicalKey); err != nil {
		return "", "", storeError(CodeOKFCompileError, "canonical key is not path-safe")
	}
	dir := okfTypeDir(rev.MemoryType)
	// Canonical key collision fallback: plain key, then --component, then
	// --short-memory-id, then fail closed (never overwrite). Collisions are
	// tracked per directory, so equal canonical keys in different memory
	// types never collide.
	candidates := []string{
		rev.CanonicalKey + ".md",
		rev.CanonicalKey + "--" + string(rev.MemoryType) + ".md",
		rev.CanonicalKey + "--" + shortMemoryID(rev.MemoryID) + ".md",
	}
	name := ""
	for _, cand := range candidates {
		if !used[dir+"/"+cand] {
			name = cand
			break
		}
	}
	if name == "" {
		return "", "", storeError(CodeOKFCompileError, "canonical key collision cannot be resolved")
	}
	// The fallback names must stay path-safe even for unusual memory ids
	// (e.g. "mem_.x" would yield a leading-dot component).
	if !isSafePageName(name) {
		return "", "", storeError(CodeOKFCompileError, "canonical key collision fallback is not path-safe")
	}
	used[dir+"/"+name] = true
	rel := "wiki/" + dir + "/" + name

	var b strings.Builder
	writeOKFFrontmatter(&b, rev, lr.evidence)
	writeOKFBody(&b, rev, lr.evidence)
	return b.String(), rel, nil
}

func shortMemoryID(id string) string {
	if strings.HasPrefix(id, "mem_") {
		return strings.TrimPrefix(id, "mem_")
	}
	return id
}

// isSafePageName reports whether a page file name is a safe single path
// component (identifier charset, no leading dot, no slash).
func isSafePageName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "/") {
		return false
	}
	return reIdentifier.MatchString(name)
}

func writeOKFFrontmatter(b *strings.Builder, rev MemoryRevision, ev MemoryEvidenceGeneration) {
	b.WriteString("---\n")
	fmt.Fprintf(b, "okf_version: %q\n", okfFrontmatterVersion)
	fmt.Fprintf(b, "type: %s\n", rev.MemoryType)
	fmt.Fprintf(b, "memory_id: %s\n", rev.MemoryID)
	fmt.Fprintf(b, "canonical_key: %s\n", rev.CanonicalKey)
	b.WriteString("title: " + yamlScalar(rev.Title) + "\n")
	b.WriteString("summary: " + yamlScalar(rev.Summary) + "\n")
	fmt.Fprintf(b, "lifecycle: %s\n", okfNotAvailable)
	fmt.Fprintf(b, "health: %s\n", okfNotAvailable)
	fmt.Fprintf(b, "usage_policy: %s\n", rev.UsagePolicy)
	fmt.Fprintf(b, "revision: %d\n", rev.Revision)
	fmt.Fprintf(b, "evidence_generation: %d\n", ev.EvidenceGeneration)
	fmt.Fprintf(b, "content_sha256: %s\n", rev.ContentSHA256)
	fmt.Fprintf(b, "evidence_set_sha256: %s\n", ev.EvidenceSetSHA256)
	b.WriteString("applies_when:\n")
	for _, c := range sortedConditions(rev.AppliesWhen) {
		b.WriteString(indent(conditionYAML(c), 2))
	}
	b.WriteString("does_not_apply_when:\n")
	for _, c := range sortedConditions(rev.DoesNotApplyWhen) {
		b.WriteString(indent(conditionYAML(c), 2))
	}
	b.WriteString("evidence:\n")
	for _, e := range sortedEvidenceRefs(ev.EvidenceRefs) {
		b.WriteString(indent(evidenceRefYAML(e), 2))
	}
	b.WriteString("relations:\n")
	for _, r := range sortedRelations(rev.Relations) {
		b.WriteString(indent(relationYAML(r), 2))
	}
	b.WriteString("---\n")
}

func writeOKFBody(b *strings.Builder, rev MemoryRevision, ev MemoryEvidenceGeneration) {
	fmt.Fprintf(b, "# %s\n\n", rev.Title)
	b.WriteString("## Summary\n\n")
	b.WriteString(rev.Summary + "\n\n")
	b.WriteString("## Applicable Conditions\n\n")
	conds := sortedConditions(rev.AppliesWhen)
	if len(conds) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, c := range conds {
			fmt.Fprintf(b, "- %s: %s.%s %s %s\n", c.ConditionID, c.Subject, c.Field, c.Operator, conditionValueYAML(c.Value))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Known Boundaries\n\n")
	boundaries := sortedConditions(rev.DoesNotApplyWhen)
	if len(boundaries) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, c := range boundaries {
			fmt.Fprintf(b, "- %s: %s.%s %s %s\n", c.ConditionID, c.Subject, c.Field, c.Operator, conditionValueYAML(c.Value))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Evidence\n\n")
	refs := sortedEvidenceRefs(ev.EvidenceRefs)
	if len(refs) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, e := range refs {
			fmt.Fprintf(b, "- %s (%s) %s\n", e.EvidenceID, e.EvidenceType, e.ContentSHA256)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Relations\n\n")
	rels := sortedRelations(rev.Relations)
	if len(rels) == 0 {
		b.WriteString("None.\n")
	} else {
		for _, r := range rels {
			fmt.Fprintf(b, "- %s -> %s %s@%d\n", r.Predicate, r.Target.MemoryType, r.Target.MemoryID, r.Target.Revision)
		}
	}
}

// ---- deterministic ordering ----

func sortedConditions(in []ApplicabilityCondition) []ApplicabilityCondition {
	out := append([]ApplicabilityCondition{}, in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ConditionID < out[j].ConditionID })
	return out
}

func sortedEvidenceRefs(in []EvidenceRef) []EvidenceRef {
	out := append([]EvidenceRef{}, in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].EvidenceType != out[j].EvidenceType {
			return out[i].EvidenceType < out[j].EvidenceType
		}
		if out[i].EvidenceID != out[j].EvidenceID {
			return out[i].EvidenceID < out[j].EvidenceID
		}
		return out[i].ContentSHA256 < out[j].ContentSHA256
	})
	return out
}

func sortedRelations(in []MemoryRelation) []MemoryRelation {
	out := append([]MemoryRelation{}, in...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Target, out[j].Target
		if out[i].Predicate != out[j].Predicate {
			return out[i].Predicate < out[j].Predicate
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.MemoryType != b.MemoryType {
			return a.MemoryType < b.MemoryType
		}
		if a.MemoryID != b.MemoryID {
			return a.MemoryID < b.MemoryID
		}
		if a.Revision != b.Revision {
			return a.Revision < b.Revision
		}
		return a.ContentSHA256 < b.ContentSHA256
	})
	return out
}

// ---- YAML emission (deterministic, hand-rolled; JSON is a YAML subset) ----

// strconvQuote renders a string as a double-quoted YAML/JSON scalar. YAML
// accepts JSON escaping, so this is safe for titles, summaries and values.
func strconvQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func jsonCompact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func yamlScalar(s string) string {
	return strconvQuote(s)
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad) + "\n"
}

func conditionValueYAML(v ConditionValue) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func conditionYAML(c ApplicabilityCondition) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- condition_id: %s\n", c.ConditionID)
	fmt.Fprintf(&sb, "  subject: %s\n", c.Subject)
	if c.SubjectRef != nil {
		sb.WriteString("  subject_ref: " + jsonCompact(c.SubjectRef) + "\n")
	} else {
		sb.WriteString("  subject_ref: null\n")
	}
	fmt.Fprintf(&sb, "  field: %s\n", c.Field)
	fmt.Fprintf(&sb, "  operator: %s\n", c.Operator)
	sb.WriteString("  value: " + conditionValueYAML(c.Value) + "\n")
	return sb.String()
}

func evidenceRefYAML(e EvidenceRef) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- scope: %s\n", e.Scope)
	fmt.Fprintf(&sb, "  evidence_type: %s\n", e.EvidenceType)
	fmt.Fprintf(&sb, "  evidence_id: %s\n", e.EvidenceID)
	fmt.Fprintf(&sb, "  content_sha256: %s\n", e.ContentSHA256)
	return sb.String()
}

func relationYAML(r MemoryRelation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- predicate: %s\n", r.Predicate)
	sb.WriteString("  target:\n")
	sb.WriteString(indent(memoryRefYAML(r.Target), 4))
	return sb.String()
}

func memoryRefYAML(r MemoryRef) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "scope: %s\n", r.Scope)
	fmt.Fprintf(&sb, "memory_type: %s\n", r.MemoryType)
	fmt.Fprintf(&sb, "memory_id: %s\n", r.MemoryID)
	fmt.Fprintf(&sb, "revision: %d\n", r.Revision)
	fmt.Fprintf(&sb, "content_sha256: %s\n", r.ContentSHA256)
	return sb.String()
}

// ---- index ----

func compileOKFIndex(scope Scope, pages []okfPage) string {
	var b strings.Builder
	b.WriteString("# OKF Wiki Index\n\n")
	fmt.Fprintf(&b, "Scope: %s\n\n", scope)
	if len(pages) == 0 {
		b.WriteString("No memories in this generation.\n")
		return b.String()
	}
	byType := map[MemoryType][]okfPage{}
	for _, p := range pages {
		byType[p.revision.MemoryType] = append(byType[p.revision.MemoryType], p)
	}
	for _, mt := range okfTypeOrder {
		group := byType[mt]
		if len(group) == 0 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].rel < group[j].rel })
		fmt.Fprintf(&b, "## %s\n\n", mt)
		for _, p := range group {
			fmt.Fprintf(&b, "- [%s](%s)\n", p.revision.Title, p.rel)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- state views ----

type okfMemoryEntry struct {
	MemoryID           string `json:"memory_id"`
	MemoryType         string `json:"memory_type"`
	CanonicalKey       string `json:"canonical_key"`
	Revision           int    `json:"revision"`
	ContentSHA256      string `json:"content_sha256"`
	EvidenceGeneration int    `json:"evidence_generation"`
	EvidenceSetSHA256  string `json:"evidence_set_sha256"`
	Page               string `json:"page"`
}

type okfMemoriesDoc struct {
	SchemaVersion int              `json:"schema_version"`
	Scope         string           `json:"scope"`
	Memories      []okfMemoryEntry `json:"memories"`
}

func compileOKFMemories(scope Scope, pages []okfPage, evidenceByKey map[string]MemoryEvidenceGeneration) ([]byte, error) {
	doc := okfMemoriesDoc{SchemaVersion: okfOutputSchemaVersion, Scope: string(scope)}
	for _, p := range pages {
		ev := evidenceByKey[p.revision.MemoryID+"\x00"+itoa(p.revision.Revision)]
		doc.Memories = append(doc.Memories, okfMemoryEntry{
			MemoryID:           p.revision.MemoryID,
			MemoryType:         string(p.revision.MemoryType),
			CanonicalKey:       p.revision.CanonicalKey,
			Revision:           p.revision.Revision,
			ContentSHA256:      p.revision.ContentSHA256,
			EvidenceGeneration: ev.EvidenceGeneration,
			EvidenceSetSHA256:  ev.EvidenceSetSHA256,
			Page:               p.rel,
		})
	}
	return json.MarshalIndent(doc, "", "  ")
}

type okfRelationRef struct {
	MemoryID   string `json:"memory_id"`
	MemoryType string `json:"memory_type"`
	Revision   int    `json:"revision"`
}

type okfRelation struct {
	Predicate string         `json:"predicate"`
	Source    okfRelationRef `json:"source"`
	Target    MemoryRef      `json:"target"`
}

type okfRelationsDoc struct {
	SchemaVersion int           `json:"schema_version"`
	Scope         string        `json:"scope"`
	Relations     []okfRelation `json:"relations"`
}

// compileOKFRelations derives the relations view. Every relation target must
// be a member of the generation's input revision set, matched exactly on
// scope, memory_type, memory_id, revision and content_sha256; anything else
// fails closed so no relation can point at a page that is not part of this
// generation.
func compileOKFRelations(scope Scope, pages []okfPage, inputs map[string]MemoryRevision) ([]byte, error) {
	doc := okfRelationsDoc{SchemaVersion: okfOutputSchemaVersion, Scope: string(scope)}
	for _, p := range pages {
		for _, r := range sortedRelations(p.revision.Relations) {
			if r.Target.Scope != scope {
				return nil, storeError(CodeOKFCompileError, "relation target crosses the generation scope")
			}
			target, ok := inputs[relationTargetKey(r.Target.Scope, r.Target.MemoryType, r.Target.MemoryID, r.Target.Revision)]
			if !ok {
				return nil, storeError(CodeOKFCompileError, "relation target is not part of this generation")
			}
			if target.ContentSHA256 != r.Target.ContentSHA256 {
				return nil, storeError(CodeOKFCompileError, "relation target content hash does not match the generation input")
			}
			doc.Relations = append(doc.Relations, okfRelation{
				Predicate: r.Predicate,
				Source: okfRelationRef{
					MemoryID:   p.revision.MemoryID,
					MemoryType: string(p.revision.MemoryType),
					Revision:   p.revision.Revision,
				},
				Target: r.Target,
			})
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}

// ---- output set hash ----

// compiledOutputHash computes the deterministic hash of a view set: the
// sorted (path, sha256) pairs over every view file. An empty set hashes to
// the empty string (no derived output). The algorithm is shared with
// generation_store.compiledOutputHash so the staged bytes and the compiled
// bytes hash identically.
func compiledOutputHash(outputs map[string][]byte) string {
	if len(outputs) == 0 {
		return ""
	}
	paths := make([]string, 0, len(outputs))
	for p := range outputs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	entries := make([]string, 0, len(paths))
	for _, p := range paths {
		entries = append(entries, p+"\n"+hashOf(outputs[p]))
	}
	return hashOf([]byte(strings.Join(entries, "\n")))
}

// okfManifestInputs derives the manifest inputs from the exact stored facts.
func okfManifestInputs(revisions []loadedRevision, evidenceByKey map[string]MemoryEvidenceGeneration, indexPolicy PolicyFact, derived *derivedInputs) ([]ManifestInput, error) {
	var out []ManifestInput
	for _, lr := range revisions {
		ft, fid, err := factIdentity(lr.revision)
		if err != nil {
			return nil, err
		}
		h, err := lr.revision.ContentHash()
		if err != nil {
			return nil, err
		}
		out = append(out, ManifestInput{
			FactType: ft, FactID: fid,
			FactSchemaVersion: factSchemaVersion(lr.revision),
			ContentSHA256:     h,
		})
		ev := evidenceByKey[lr.revision.MemoryID+"\x00"+itoa(lr.revision.Revision)]
		eft, efid, err := factIdentity(ev)
		if err != nil {
			return nil, err
		}
		eh, err := ev.ContentHash()
		if err != nil {
			return nil, err
		}
		out = append(out, ManifestInput{
			FactType: eft, FactID: efid,
			FactSchemaVersion: factSchemaVersion(ev),
			ContentSHA256:     eh,
		})
	}
	ft, fid, err := factIdentity(indexPolicy)
	if err != nil {
		return nil, err
	}
	h, err := indexPolicy.ContentHash()
	if err != nil {
		return nil, err
	}
	out = append(out, ManifestInput{FactType: ft, FactID: fid, FactSchemaVersion: factSchemaVersion(indexPolicy), ContentSHA256: h})
	for _, fact := range derivedFacts(derived) {
		ft, fid, err := factIdentity(fact)
		if err != nil {
			return nil, err
		}
		h, err := fact.ContentHash()
		if err != nil {
			return nil, err
		}
		out = append(out, ManifestInput{FactType: ft, FactID: fid, FactSchemaVersion: factSchemaVersion(fact), ContentSHA256: h})
	}
	return dedupeManifestInputs(out)
}

func derivedFacts(in *derivedInputs) []Fact {
	var out []Fact
	for _, revisions := range in.revisions {
		for _, fact := range revisions {
			out = append(out, fact)
		}
	}
	for _, generations := range in.evidence {
		for _, fact := range generations {
			out = append(out, fact)
		}
	}
	for _, fact := range in.judgments {
		out = append(out, fact)
	}
	for _, fact := range in.governance {
		out = append(out, fact)
	}
	for _, fact := range in.usages {
		out = append(out, fact)
	}
	for _, fact := range in.outcomes {
		out = append(out, fact)
	}
	return out
}
