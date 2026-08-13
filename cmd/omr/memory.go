package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	mem "github.com/mchenziyi/oh-my-reasonix/internal/memory"
)

type episodicContextDocument struct {
	SchemaVersion int                       `json:"schema_version"`
	Project       *mem.EpisodicScopeContext `json:"project"`
	Global        *mem.EpisodicScopeContext `json:"global"`
}

func runMemory(args []string) error {
	if len(args) == 1 && args[0] == "history" {
		return runMemoryHistory(nil)
	}
	if len(args) == 1 && args[0] == "usage" {
		return runMemoryUsageHistory(nil)
	}
	if len(args) < 2 {
		return errors.New("memory requires an episodic, usage or outcome command")
	}
	if args[0] == "usage" {
		if len(args) > 1 && args[1] != "capture" {
			return runMemoryUsageHistory(args[1:])
		}
		if args[1] != "capture" {
			return errors.New("memory usage requires capture")
		}
		return runMemoryUsageCapture(args[2:])
	}
	if args[0] == "outcome" {
		switch args[1] {
		case "capture":
			return runMemoryOutcomeCapture(args[2:])
		case "override":
			return runMemoryOutcomeOverride(args[2:])
		default:
			return errors.New("memory outcome requires capture or override")
		}
	}
	if args[0] == "get" {
		return runMemoryGet(args[1:])
	}
	if args[0] == "doctor" {
		return runMemoryConsistencyDoctor(args[1:])
	}
	if args[0] == "status" {
		return runMemoryStatus(args[1:])
	}
	if args[0] == "report" {
		return runMemoryReport(args[1:])
	}
	if args[0] == "web" {
		if len(args) < 2 {
			return errors.New("memory web requires export or audit")
		}
		switch args[1] {
		case "export":
			return runMemoryWebExport(args[2:])
		case "audit":
			return runMemoryWebAudit(args[2:])
		case "action":
			if len(args) < 3 {
				return errors.New("memory web action requires validate or apply")
			}
			switch args[2] {
			case "validate":
				return runMemoryWebActionValidate(args[3:])
			case "apply":
				return runMemoryWebActionApply(args[3:])
			default:
				return errors.New("memory web action requires validate or apply")
			}
		case "serve":
			return runMemoryWebServe(args[2:])
		default:
			return errors.New("memory web requires export, audit, action or serve")
		}
	}
	if args[0] == "compile" {
		return runMemoryCompile(args[1:])
	}
	if args[0] == "index" {
		if len(args) < 2 {
			return errors.New("memory index requires rebuild, doctor or publish")
		}
		switch args[1] {
		case "rebuild":
			return runMemoryIndexRebuild(args[2:])
		case "doctor":
			return runMemoryIndexDoctor(args[2:])
		case "publish":
			return runMemoryIndexPublish(args[2:])
		default:
			return errors.New("memory index requires rebuild, doctor or publish")
		}
	}
	if args[0] == "benchmark" {
		if len(args) < 2 || args[1] != "paired" {
			return errors.New("memory benchmark requires paired")
		}
		return runMemoryPairedBenchmark(args[2:])
	}
	if args[0] == "retrieval" {
		if len(args) < 2 || args[1] != "audit" {
			return errors.New("memory retrieval requires audit")
		}
		return runMemoryRetrievalAudit(args[2:])
	}
	if args[0] == "episode" {
		if len(args) < 2 {
			return errors.New("memory episode requires list or show")
		}
		switch args[1] {
		case "list":
			return runMemoryEpisodeList(args[2:])
		case "show":
			return runMemoryEpisodeShow(args[2:])
		default:
			return errors.New("memory episode requires list or show")
		}
	}
	if args[0] == "migration" {
		return runMemoryMigration(args[1:])
	}
	if args[0] == "generalize" {
		if len(args) < 2 || args[1] != "apply" {
			return errors.New("memory generalize requires apply")
		}
		return runMemoryGeneralizeApply(args[2:])
	}
	if args[0] == "promotion" {
		if len(args) < 2 {
			return errors.New("memory promotion requires apply or candidate")
		}
		if args[1] == "candidate" {
			if len(args) < 3 || (args[2] != "put" && args[2] != "apply") {
				return errors.New("memory promotion candidate requires put or apply")
			}
			if args[2] == "apply" {
				return runMemoryPromotionCandidateApply(args[3:])
			}
			return runMemoryPromotionCandidatePut(args[3:])
		}
		if args[1] == "generation" {
			if len(args) < 3 || args[2] != "publish" {
				return errors.New("memory promotion generation requires publish")
			}
			return runMemoryPromotionGenerationPublish(args[3:])
		}
		if args[1] != "apply" {
			return errors.New("memory promotion requires apply or candidate")
		}
		return runMemoryPromotionApply(args[2:])
	}
	if args[0] == "rollback" {
		return runMemoryRollback(args[1:])
	}
	if args[0] == "repair" {
		return runMemoryRepair(args[1:])
	}
	if args[0] == "list" {
		return runMemoryList(args[1:])
	}
	if args[0] == "show" {
		return runMemoryShow(args[1:])
	}
	if args[0] == "history" {
		return runMemoryHistory(args[1:])
	}
	if args[0] == "usage" {
		return runMemoryUsageHistory(args[1:])
	}
	switch args[0] {
	case "pin", "unpin", "freeze", "unfreeze", "archive":
		return runMemoryGovernance(args[0], args[1:])
	}
	if args[0] != "episodic" {
		return errors.New("memory requires get, list, show, benchmark, retrieval, migration, rollback, repair, episodic, usage or outcome command")
	}
	switch args[1] {
	case "context":
		return runMemoryContext(args[2:])
	case "index":
		return runMemoryIndex(args[2:])
	case "card":
		return runMemoryCard(args[2:])
	case "validate-receipt":
		return runMemoryReceipt(args[2:])
	case "doctor":
		return runMemoryDoctor(args[2:])
	default:
		return fmt.Errorf("unknown memory episodic subcommand %q", args[1])
	}
}

type promotionCandidateSourceInput struct {
	Ref               mem.MemoryRef `json:"ref"`
	ProjectDir        string        `json:"project_dir"`
	FamilyFingerprint string        `json:"family_fingerprint"`
}

type memoryCompileRequestInput struct {
	Scope            mem.Scope                `json:"scope"`
	BaseGeneration   *string                  `json:"base_generation"`
	IndexPolicyRef   mem.PolicyRef            `json:"index_policy_ref"`
	EvaluationTime   string                   `json:"evaluation_time"`
	DerivationInputs []mem.ManifestInput      `json:"derivation_inputs"`
	Revisions        []memoryRevisionRefInput `json:"revisions"`
	Evidence         []memoryEvidenceRefInput `json:"evidence"`
}

type memoryRevisionRefInput struct {
	MemoryID      string `json:"memory_id"`
	Revision      int    `json:"revision"`
	ContentSHA256 string `json:"content_sha256"`
}

type memoryEvidenceRefInput struct {
	MemoryID           string `json:"memory_id"`
	Revision           int    `json:"revision"`
	EvidenceGeneration int    `json:"evidence_generation"`
	EvidenceSetSHA256  string `json:"evidence_set_sha256"`
}

type memoryCompileOutput struct {
	Scope                   mem.Scope `json:"scope"`
	CompilerVersion         string    `json:"compiler_version"`
	CanonicalizationVersion int       `json:"canonicalization_version"`
	EvaluationTime          string    `json:"evaluation_time"`
	InputCount              int       `json:"input_count"`
	OutputPaths             []string  `json:"output_paths"`
	CompiledSHA256          string    `json:"compiled_sha256"`
}

type memoryIndexRebuildOutput struct {
	Scope          mem.Scope `json:"scope"`
	EvaluationTime string    `json:"evaluation_time"`
	InputCount     int       `json:"input_count"`
	RootEntries    int       `json:"root_entries"`
	LocalShards    int       `json:"local_shards"`
}

type promotionCandidateApplyInput struct {
	Candidate mem.GlobalPromotionCandidate    `json:"candidate"`
	Sources   []promotionCandidateSourceInput `json:"sources"`
	Target    mem.MemoryRevision              `json:"target"`
}

type promotionGenerationPublishInput struct {
	Candidate      mem.GlobalPromotionCandidate    `json:"candidate"`
	Sources        []promotionCandidateSourceInput `json:"sources"`
	Target         mem.MemoryRevision              `json:"target"`
	Compile        mem.OKFCompileRequest           `json:"compile"`
	EvaluationTime time.Time                       `json:"evaluation_time"`
	IdempotencyKey string                          `json:"idempotency_key"`
	BaseGeneration *string                         `json:"base_generation"`
}

func runMemoryPromotionGenerationPublish(args []string) error {
	fs := flag.NewFlagSet("memory promotion generation publish", flag.ContinueOnError)
	global := fs.String("global-dir", "", "global memory directory")
	input := fs.String("input", "", "promotion generation request JSON file")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *global == "" || *input == "" {
		return errors.New("promotion generation publish requires --global-dir and --input")
	}
	b, err := readBoundedJSONFile(*input)
	if err != nil {
		return err
	}
	var in promotionGenerationPublishInput
	if err := strictJSON(b, &in); err != nil || len(in.Sources) == 0 || in.IdempotencyKey == "" || in.EvaluationTime.IsZero() {
		return errors.New("promotion generation request JSON is invalid")
	}
	globalStore, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
	if err != nil {
		return err
	}
	sources := make([]mem.PromotionCandidateSource, 0, len(in.Sources))
	for _, item := range in.Sources {
		if item.ProjectDir == "" {
			return errors.New("promotion generation source project directory is required")
		}
		store, err := openExistingMemoryStore(item.ProjectDir, mem.ScopeProject)
		if err != nil {
			return err
		}
		sources = append(sources, mem.PromotionCandidateSource{Ref: item.Ref, Store: store, FamilyFingerprint: item.FamilyFingerprint})
	}
	result, err := mem.PublishPromotionGeneration(context.Background(), mem.PromotionGenerationRequest{Candidate: in.Candidate, Sources: sources, Target: in.Target, Global: globalStore, Compile: in.Compile, EvaluationTime: in.EvaluationTime, IdempotencyKey: in.IdempotencyKey, BaseGeneration: in.BaseGeneration})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryPromotionCandidateApply(args []string) error {
	fs := flag.NewFlagSet("memory promotion candidate apply", flag.ContinueOnError)
	global := fs.String("global-dir", "", "global memory directory")
	input := fs.String("input", "", "candidate apply request JSON file")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *global == "" || *input == "" {
		return errors.New("promotion candidate apply requires --global-dir and --input")
	}
	b, err := readBoundedJSONFile(*input)
	if err != nil {
		return err
	}
	var in promotionCandidateApplyInput
	if err := strictJSON(b, &in); err != nil || len(in.Sources) == 0 {
		return errors.New("promotion candidate apply JSON is invalid")
	}
	globalStore, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
	if err != nil {
		return err
	}
	sources := make([]mem.PromotionCandidateSource, 0, len(in.Sources))
	for _, item := range in.Sources {
		if item.ProjectDir == "" {
			return errors.New("promotion candidate source project directory is required")
		}
		store, err := openExistingMemoryStore(item.ProjectDir, mem.ScopeProject)
		if err != nil {
			return err
		}
		sources = append(sources, mem.PromotionCandidateSource{Ref: item.Ref, Store: store, FamilyFingerprint: item.FamilyFingerprint})
	}
	result, err := mem.ApplyPromotionCandidate(context.Background(), mem.PromotionCandidateApplyRequest{Candidate: in.Candidate, Sources: sources, Target: in.Target, Global: globalStore})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryPromotionCandidatePut(args []string) error {
	fs := flag.NewFlagSet("memory promotion candidate put", flag.ContinueOnError)
	global := fs.String("global-dir", "", "global memory directory")
	input := fs.String("input", "", "GlobalPromotionCandidate JSON file")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *global == "" || *input == "" {
		return errors.New("promotion candidate put requires --global-dir and --input")
	}
	b, err := readBoundedJSONFile(*input)
	if err != nil {
		return err
	}
	candidate, err := mem.DecodeStrict[mem.GlobalPromotionCandidate](b)
	if err != nil {
		return errors.New("promotion candidate JSON is invalid")
	}
	store, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
	if err != nil {
		return err
	}
	result, err := store.Put(context.Background(), candidate)
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryPromotionApply(args []string) error {
	fs := flag.NewFlagSet("memory promotion apply", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	planPath := fs.String("plan", "", "PromotionPlan JSON file")
	policyPath := fs.String("policy", "", "Trust PolicyFact JSON file")
	targetPath := fs.String("target", "", "global MemoryRevision JSON file")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *global == "" || *planPath == "" || *policyPath == "" || *targetPath == "" {
		return errors.New("promotion apply requires --global-dir, --plan, --policy and --target")
	}
	planBytes, err := readBoundedJSONFile(*planPath)
	if err != nil {
		return err
	}
	policyBytes, err := readBoundedJSONFile(*policyPath)
	if err != nil {
		return err
	}
	targetBytes, err := readBoundedJSONFile(*targetPath)
	if err != nil {
		return err
	}
	var plan mem.PromotionPlan
	if err := strictJSON(planBytes, &plan); err != nil {
		return errors.New("promotion plan JSON is invalid")
	}
	var policy mem.PolicyFact
	if err := strictJSON(policyBytes, &policy); err != nil {
		return errors.New("promotion policy JSON is invalid")
	}
	var target mem.MemoryRevision
	if err := strictJSON(targetBytes, &target); err != nil {
		return errors.New("promotion target JSON is invalid")
	}
	projectStore, err := openExistingMemoryStore(*project, mem.ScopeProject)
	if err != nil {
		return err
	}
	globalStore, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
	if err != nil {
		return err
	}
	result, err := mem.ApplyPromotionPlan(context.Background(), projectStore, globalStore, plan, policy, target)
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryGeneralizeApply(args []string) error {
	fs := flag.NewFlagSet("memory generalize apply", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	planPath := fs.String("plan", "", "GeneralizePlan JSON file")
	targetPath := fs.String("target", "", "global MemoryRevision JSON file")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *global == "" || *planPath == "" || *targetPath == "" {
		return errors.New("generalize apply requires --global-dir, --plan and --target")
	}
	planBytes, err := readBoundedJSONFile(*planPath)
	if err != nil {
		return err
	}
	targetBytes, err := readBoundedJSONFile(*targetPath)
	if err != nil {
		return err
	}
	var plan mem.GeneralizePlan
	if err := strictJSON(planBytes, &plan); err != nil {
		return errors.New("generalize plan JSON is invalid")
	}
	var target mem.MemoryRevision
	if err := strictJSON(targetBytes, &target); err != nil {
		return errors.New("generalize target JSON is invalid")
	}
	projectStore, err := openExistingMemoryStore(*project, mem.ScopeProject)
	if err != nil {
		return err
	}
	globalStore, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
	if err != nil {
		return err
	}
	result, err := mem.ApplyGeneralizePlan(context.Background(), projectStore, globalStore, plan, target)
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryReport(args []string) error {
	fs := flag.NewFlagSet("memory report", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	nowText := fs.String("now", "", "explicit evaluation timestamp (RFC3339)")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nowText == "" {
		return errors.New("now is required for deterministic memory report")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil {
		return errors.New("now must be a valid RFC3339 timestamp")
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	report, err := mem.BuildLifecycleReport(context.Background(), mem.LifecycleReportRequest{Store: store, Scope: sc, Now: now.UTC()})
	if err != nil {
		return err
	}
	return writeJSONOutput(report)
}

func runMemoryCompile(args []string) error {
	fs := flag.NewFlagSet("memory compile", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	requestPath := fs.String("request", "", "strict OKF compile request JSON")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *requestPath == "" {
		return errors.New("compile requires --request")
	}
	b, err := readBoundedJSONFile(*requestPath)
	if err != nil {
		return errors.New("compile request is unavailable")
	}
	var input memoryCompileRequestInput
	if err := strictJSON(b, &input); err != nil {
		return errors.New("compile request is invalid")
	}
	scope := mem.Scope(*scopeText)
	if input.Scope != "" && input.Scope != scope {
		return errors.New("compile request scope does not match --scope")
	}
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	now, err := time.Parse(time.RFC3339Nano, input.EvaluationTime)
	if err != nil || now.IsZero() {
		return errors.New("compile request evaluation_time must be a valid RFC3339 timestamp")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	revisions := make([]mem.MemoryRevisionRef, 0, len(input.Revisions))
	for _, ref := range input.Revisions {
		revisions = append(revisions, mem.MemoryRevisionRef{MemoryID: ref.MemoryID, Revision: ref.Revision, ContentSHA256: ref.ContentSHA256})
	}
	evidence := make([]mem.MemoryEvidenceRef, 0, len(input.Evidence))
	for _, ref := range input.Evidence {
		evidence = append(evidence, mem.MemoryEvidenceRef{MemoryID: ref.MemoryID, Revision: ref.Revision, EvidenceGeneration: ref.EvidenceGeneration, EvidenceSetSHA256: ref.EvidenceSetSHA256})
	}
	result, err := mem.CompileOKF(context.Background(), store, mem.OKFCompileRequest{
		Scope: scope, BaseGeneration: input.BaseGeneration, IndexPolicyRef: input.IndexPolicyRef,
		EvaluationTime: now.UTC(), DerivationInputs: input.DerivationInputs, Revisions: revisions, Evidence: evidence,
	})
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(result.Outputs))
	for path := range result.Outputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return writeJSONOutput(memoryCompileOutput{Scope: scope, CompilerVersion: mem.OKFCompilerVersion, CanonicalizationVersion: mem.OKFCanonicalizationVersion, EvaluationTime: now.UTC().Format(time.RFC3339Nano), InputCount: len(result.Inputs), OutputPaths: paths, CompiledSHA256: result.CompiledSHA256})
}

func runMemoryIndexRebuild(args []string) error {
	fs := flag.NewFlagSet("memory index rebuild", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	requestPath := fs.String("request", "", "strict index rebuild request JSON")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *requestPath == "" {
		return errors.New("index rebuild requires --request")
	}
	b, err := readBoundedJSONFile(*requestPath)
	if err != nil {
		return errors.New("index rebuild request is unavailable")
	}
	var input memoryCompileRequestInput
	if err := strictJSON(b, &input); err != nil {
		return errors.New("index rebuild request is invalid")
	}
	scope := mem.Scope(*scopeText)
	if input.Scope != "" && input.Scope != scope {
		return errors.New("index rebuild request scope does not match --scope")
	}
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	now, err := time.Parse(time.RFC3339Nano, input.EvaluationTime)
	if err != nil || now.IsZero() {
		return errors.New("index rebuild request evaluation_time must be a valid RFC3339 timestamp")
	}
	if len(input.DerivationInputs) == 0 || len(input.Revisions) == 0 {
		return errors.New("index rebuild request requires explicit inputs")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	revisions := make([]mem.MemoryRevisionRef, 0, len(input.Revisions))
	for _, ref := range input.Revisions {
		revisions = append(revisions, mem.MemoryRevisionRef{MemoryID: ref.MemoryID, Revision: ref.Revision, ContentSHA256: ref.ContentSHA256})
	}
	result, err := mem.RebuildIndexPreview(context.Background(), store, mem.IndexRebuildRequest{Scope: scope, EvaluationTime: now.UTC(), IndexPolicyRef: input.IndexPolicyRef, DerivationInputs: input.DerivationInputs, Revisions: revisions})
	if err != nil {
		return err
	}
	return writeJSONOutput(memoryIndexRebuildOutput{Scope: scope, EvaluationTime: now.UTC().Format(time.RFC3339Nano), InputCount: len(revisions), RootEntries: len(result.RootIndex.Entries), LocalShards: len(result.LocalIndex.Shards)})
}

func runMemoryIndexPublish(args []string) error {
	fs := flag.NewFlagSet("memory index publish", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	requestPath := fs.String("request", "", "strict OKF compile request JSON")
	idempotencyKey := fs.String("idempotency-key", "", "idempotency key")
	dryRun := fs.Bool("dry-run", false, "compile without publishing")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *requestPath == "" {
		return errors.New("index publish requires --request")
	}
	b, err := readBoundedJSONFile(*requestPath)
	if err != nil {
		return errors.New("index publish request is unavailable")
	}
	var input memoryCompileRequestInput
	if err := strictJSON(b, &input); err != nil {
		return errors.New("index publish request is invalid")
	}
	scope := mem.Scope(*scopeText)
	if input.Scope != "" && input.Scope != scope {
		return errors.New("index publish request scope does not match --scope")
	}
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	now, err := time.Parse(time.RFC3339Nano, input.EvaluationTime)
	if err != nil || now.IsZero() {
		return errors.New("index publish request evaluation_time must be a valid RFC3339 timestamp")
	}
	if !*dryRun && *idempotencyKey == "" {
		return errors.New("index publish requires --idempotency-key")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	revisions := make([]mem.MemoryRevisionRef, 0, len(input.Revisions))
	for _, ref := range input.Revisions {
		revisions = append(revisions, mem.MemoryRevisionRef{MemoryID: ref.MemoryID, Revision: ref.Revision, ContentSHA256: ref.ContentSHA256})
	}
	evidence := make([]mem.MemoryEvidenceRef, 0, len(input.Evidence))
	for _, ref := range input.Evidence {
		evidence = append(evidence, mem.MemoryEvidenceRef{MemoryID: ref.MemoryID, Revision: ref.Revision, EvidenceGeneration: ref.EvidenceGeneration, EvidenceSetSHA256: ref.EvidenceSetSHA256})
	}
	request := mem.OKFCompileRequest{Scope: scope, BaseGeneration: input.BaseGeneration, IndexPolicyRef: input.IndexPolicyRef, EvaluationTime: now.UTC(), DerivationInputs: input.DerivationInputs, Revisions: revisions, Evidence: evidence}
	if *dryRun {
		compiled, err := mem.CompileOKF(context.Background(), store, request)
		if err != nil {
			return err
		}
		return writeJSONOutput(struct {
			Scope          mem.Scope `json:"scope"`
			DryRun         bool      `json:"dry_run"`
			InputCount     int       `json:"input_count"`
			CompiledSHA256 string    `json:"compiled_sha256"`
		}{scope, true, len(compiled.Inputs), compiled.CompiledSHA256})
	}
	result, err := mem.PublishIndexGeneration(context.Background(), store, mem.IndexPublishRequest{OKF: request, IdempotencyKey: *idempotencyKey})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

type memoryIndexDoctorRequest struct {
	Scope          mem.Scope     `json:"scope"`
	IndexPolicyRef mem.PolicyRef `json:"index_policy_ref"`
}

func runMemoryIndexDoctor(args []string) error {
	fs := flag.NewFlagSet("memory index doctor", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	indexPath := fs.String("index", "", "index tree JSON file")
	requestPath := fs.String("request", "", "strict index doctor request JSON")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" || *requestPath == "" {
		return errors.New("index doctor requires --index and --request")
	}
	b, err := readBoundedJSONFile(*requestPath)
	if err != nil {
		return errors.New("index doctor request is unavailable")
	}
	var input memoryIndexDoctorRequest
	if err := strictJSON(b, &input); err != nil {
		return errors.New("index doctor request is invalid")
	}
	scope := mem.Scope(*scopeText)
	if input.Scope != "" && input.Scope != scope {
		return errors.New("index doctor request scope does not match --scope")
	}
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	policy, err := mem.NewPolicyStore(store).GetPolicy(context.Background(), input.IndexPolicyRef)
	if err != nil {
		return err
	}
	if policy.Config.Index == nil {
		return errors.New("index policy config is missing")
	}
	data, err := readBoundedJSONFile(*indexPath)
	if err != nil {
		return errors.New("index tree is unavailable")
	}
	diagnostics := mem.DiagnoseIndexTree(data, *policy.Config.Index)
	return writeJSONOutput(map[string]any{"scope": scope, "healthy": len(diagnostics) == 0, "diagnostics": diagnostics})
}

func runMemoryPairedBenchmark(args []string) error {
	fs := flag.NewFlagSet("memory benchmark paired", flag.ContinueOnError)
	fixture := fs.String("paired-fixture", "", "paired benchmark fixture JSON")
	output := fs.String("output", "", "optional JSON report path")
	_ = fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fixture == "" {
		return errors.New("memory benchmark paired requires --paired-fixture")
	}
	report, err := mem.RunPairedBenchmarkFixture(context.Background(), *fixture)
	if err != nil {
		return err
	}
	if *output != "" {
		return writeJSONValue(*output, "paired benchmark", report)
	}
	return writeJSONOutput(report)
}

func runMemoryRetrievalAudit(args []string) error {
	args = normalizeLeadingTargetArgs(args)
	fs := flag.NewFlagSet("memory retrieval audit", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	nowText := fs.String("now", "", "explicit RFC3339 evaluation time")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("retrieval audit requires evaluation id")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil || *nowText == "" {
		return errors.New("retrieval audit requires a valid --now timestamp")
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	result, err := mem.ValidateRetrievalEvaluation(context.Background(), store, mem.RetrievalEvaluationRequest{Scope: sc, EvaluationID: fs.Arg(0), ProjectStore: store, Now: now.UTC()})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryMigration(args []string) error {
	if len(args) == 0 || (args[0] != "preview" && args[0] != "doctor" && args[0] != "copy" && args[0] != "apply") {
		return errors.New("memory migration requires preview, doctor, copy or apply")
	}
	action := args[0]
	fs := flag.NewFlagSet("memory migration "+action, flag.ContinueOnError)
	sourceDir := fs.String("source-dir", "", "source project directory")
	targetDir := fs.String("target-dir", "", "target project directory")
	scopeText := fs.String("scope", "project", "project or global")
	generationID := fs.String("generation-id", "", "source generation id")
	idempotency := fs.String("idempotency-key", "", "migration idempotency key")
	planFile := fs.String("plan-file", "", "persisted migration preview JSON")
	output := fs.String("output", "", "optional JSON output path")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *sourceDir == "" || *targetDir == "" || (*generationID == "" && *planFile == "") {
		return errors.New("migration requires --source-dir, --target-dir and --generation-id or --plan-file")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("migration scope is invalid")
	}
	source, err := openExistingMemoryStore(*sourceDir, scope)
	if err != nil {
		return err
	}
	target, err := openExistingMemoryStore(*targetDir, scope)
	if err != nil {
		return err
	}
	var plan mem.MigrationPlan
	if *planFile != "" {
		data, readErr := readBoundedJSONFile(*planFile)
		if readErr != nil {
			return errors.New("migration plan file is unavailable")
		}
		if readErr := strictJSON(data, &plan); readErr != nil {
			return errors.New("migration plan file is invalid")
		}
		if readErr := plan.Validate(); readErr != nil {
			return readErr
		}
		if plan.SourceScope != scope || plan.TargetScope != scope {
			return errors.New("migration plan scope does not match command")
		}
	} else {
		plan, err = mem.BuildMigrationPlanFromStores(context.Background(), source, target, *generationID)
		if err != nil {
			return err
		}
	}
	if action == "preview" {
		if *output != "" {
			return writeJSONValue(*output, "migration plan", plan)
		}
		return writeJSONOutput(plan)
	}
	if action == "doctor" {
		report, err := mem.CheckMigrationReadiness(context.Background(), source, target, plan)
		if err != nil {
			return err
		}
		if *output != "" {
			return writeJSONValue(*output, "migration doctor", report)
		}
		return writeJSONOutput(report)
	}
	if action == "copy" {
		result, err := mem.ApplyMigrationCopy(context.Background(), source, target, mem.MigrationCopyRequest{Plan: plan})
		if err != nil {
			return err
		}
		if *output != "" {
			return writeJSONValue(*output, "migration copy", result)
		}
		return writeJSONOutput(result)
	}
	if *idempotency == "" {
		return errors.New("migration apply requires --idempotency-key")
	}
	result, err := mem.ApplyMigration(context.Background(), source, target, mem.MigrationApplyRequest{Plan: plan, IdempotencyKey: *idempotency})
	if err != nil {
		return err
	}
	if *output != "" {
		return writeJSONValue(*output, "migration result", result)
	}
	return writeJSONOutput(result)
}

func runMemoryRollback(args []string) error {
	args = normalizeLeadingTargetArgs(args)
	fs := flag.NewFlagSet("memory rollback", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	operator := fs.String("operator", "", "operator identity")
	reason := fs.String("reason", "", "rollback reason")
	nowText := fs.String("now", "", "explicit RFC3339 timestamp")
	idempotency := fs.String("idempotency-key", "", "rollback idempotency key")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *operator == "" || *reason == "" || *nowText == "" || *idempotency == "" {
		return errors.New("rollback requires target id, --operator, --reason, --now and --idempotency-key")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil {
		return errors.New("rollback requires a valid --now timestamp")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("rollback scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("rollback scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	plan, err := mem.BuildRollbackPlan(context.Background(), mem.NewGenerationStore(store), fs.Arg(0))
	if err != nil {
		return err
	}
	if !plan.Eligible {
		return errors.New("rollback target is not eligible")
	}
	result, err := mem.ApplyRollbackPlan(context.Background(), mem.NewGenerationStore(store), mem.RollbackRequest{Plan: plan, Operator: *operator, Reason: *reason, Now: now, IdempotencyKey: *idempotency})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryRepair(args []string) error {
	fs := flag.NewFlagSet("memory repair", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("repair scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("repair scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	plan, err := mem.BuildRepairPlan(context.Background(), mem.NewGenerationStore(store))
	if err != nil {
		return err
	}
	return writeJSONOutput(plan)
}

func runMemoryList(args []string) error {
	fs := flag.NewFlagSet("memory list", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	kindText := fs.String("kind", "memory-revisions", "fact kind")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("list scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("list scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	keys, err := store.List(context.Background(), mem.FactKind(*kindText))
	if err != nil {
		return err
	}
	return writeJSONOutput(struct {
		Scope mem.Scope    `json:"scope"`
		Kind  mem.FactKind `json:"kind"`
		Keys  []string     `json:"keys"`
	}{scope, mem.FactKind(*kindText), keys})
}

func runMemoryWebExport(args []string) error {
	fs := flag.NewFlagSet("memory web export", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global directory")
	scopeText := fs.String("scope", "project", "project or global")
	nowText := fs.String("now", "", "explicit evaluation timestamp")
	output := fs.String("output", "", "output HTML path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("web export requires --output")
	}
	if *nowText == "" {
		return errors.New("web export now is required")
	}
	now, err := time.Parse(time.RFC3339, *nowText)
	if err != nil {
		return errors.New("web export now is invalid")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("web export scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("web export scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	data, err := mem.BuildMemoryWebExport(context.Background(), store, now)
	if err != nil {
		return err
	}
	if err := writeImmutableMemoryExport(*output, data); err != nil {
		return err
	}
	return writeJSONOutput(struct {
		Scope  mem.Scope `json:"scope"`
		Output string    `json:"output"`
	}{scope, filepath.Clean(*output)})
}

func runMemoryWebAudit(args []string) error {
	fs := flag.NewFlagSet("memory web audit", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global directory")
	scopeText := fs.String("scope", "project", "project or global")
	nowText := fs.String("now", "", "explicit evaluation timestamp")
	output := fs.String("output", "", "output HTML path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("web audit requires --output")
	}
	if *nowText == "" {
		return errors.New("web audit now is required")
	}
	now, err := time.Parse(time.RFC3339, *nowText)
	if err != nil {
		return errors.New("web audit now is invalid")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("web audit scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("web audit scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	data, err := mem.BuildMemoryAuditWebExport(context.Background(), store, now)
	if err != nil {
		return err
	}
	if err := writeImmutableMemoryExport(*output, data); err != nil {
		return err
	}
	return writeJSONOutput(struct {
		Scope  mem.Scope `json:"scope"`
		Output string    `json:"output"`
	}{scope, filepath.Clean(*output)})
}

func runMemoryWebActionValidate(args []string) error {
	fs := flag.NewFlagSet("memory web action validate", flag.ContinueOnError)
	input := fs.String("input", "", "action JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := readBoundedJSONFile(*input)
	if err != nil {
		return err
	}
	var action mem.WebManagementAction
	if err := strictJSON(data, &action); err != nil {
		return errors.New("web action JSON is invalid")
	}
	if err := action.Validate(); err != nil {
		return errors.New("web action is invalid")
	}
	hash, err := action.ContentHash()
	if err != nil {
		return errors.New("web action is invalid")
	}
	return writeJSONOutput(struct {
		Valid      bool   `json:"valid"`
		ActionID   string `json:"action_id"`
		ContentSHA string `json:"content_sha256"`
	}{true, action.ActionID, hash})
}

func runMemoryWebActionApply(args []string) error {
	fs := flag.NewFlagSet("memory web action apply", flag.ContinueOnError)
	input := fs.String("input", "", "action JSON file")
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global directory")
	confirm := fs.Bool("confirm", false, "explicit second confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := readBoundedJSONFile(*input)
	if err != nil {
		return err
	}
	var action mem.WebManagementAction
	if err := strictJSON(data, &action); err != nil || action.Validate() != nil {
		return errors.New("web action is invalid")
	}
	dir := *project
	if action.Scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("web action scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, action.Scope)
	if err != nil {
		return err
	}
	result, err := mem.ApplyWebManagementAction(context.Background(), store, action, *confirm)
	if err != nil {
		return err
	}
	return writeJSONOutput(struct {
		Status  string `json:"status"`
		EventID string `json:"event_id"`
	}{result.Status.String(), result.Event.EventID})
}

func runMemoryWebServe(args []string) error {
	fs := flag.NewFlagSet("memory web serve", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global directory")
	scopeText := fs.String("scope", "project", "project or global")
	nowText := fs.String("now", "", "explicit evaluation timestamp")
	listen := fs.String("listen", "127.0.0.1:0", "loopback listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nowText == "" {
		return errors.New("web serve now is required")
	}
	now, err := time.Parse(time.RFC3339, *nowText)
	if err != nil {
		return errors.New("web serve now is invalid")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return errors.New("web serve only permits loopback listen addresses")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("web serve scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("web serve scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	handler, err := mem.NewMemoryWebHandler(store, now)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return errors.New("web serve listener is unavailable")
	}
	defer listener.Close()
	if err := writeJSONOutput(map[string]any{"url": "http://" + listener.Addr().String(), "scope": scope}); err != nil {
		return err
	}
	return http.Serve(listener, handler)
}

func writeImmutableMemoryExport(path string, data []byte) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("web export output is invalid")
	}
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("web export output is unsafe")
		}
		old, err := os.ReadFile(clean)
		if err != nil {
			return errors.New("web export output is unavailable")
		}
		if !bytes.Equal(old, data) {
			return errors.New("web export output already exists with different content")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("web export output is unavailable")
	}
	f, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("web export output is unavailable")
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return errors.New("web export output write failed")
	}
	if err := f.Sync(); err != nil {
		return errors.New("web export output sync failed")
	}
	return nil
}

func runMemoryShow(args []string) error {
	args = normalizeLeadingTargetArgs(args)
	fs := flag.NewFlagSet("memory show", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	kindText := fs.String("kind", "", "fact kind")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *kindText == "" {
		return errors.New("show requires --kind and one fact key")
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return errors.New("show scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("show scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return err
	}
	data, err := store.Get(context.Background(), mem.FactKind(*kindText), fs.Arg(0))
	if err != nil {
		return err
	}
	return writeJSONOutput(struct {
		Scope mem.Scope       `json:"scope"`
		Kind  mem.FactKind    `json:"kind"`
		Key   string          `json:"key"`
		Fact  json.RawMessage `json:"fact"`
	}{scope, mem.FactKind(*kindText), fs.Arg(0), json.RawMessage(data)})
}

type memoryRevisionHistoryEntry struct {
	MemoryID      string          `json:"memory_id"`
	Revision      int             `json:"revision"`
	MemoryType    mem.MemoryType  `json:"memory_type"`
	CanonicalKey  string          `json:"canonical_key"`
	UsagePolicy   mem.UsagePolicy `json:"usage_policy"`
	ContentSHA256 string          `json:"content_sha256"`
	CreatedAt     string          `json:"created_at"`
}

func openMemoryScope(args []string, name string) (*string, *mem.FactStore, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project memory directory")
	global := fs.String("global-dir", "", "global memory directory")
	scopeText := fs.String("scope", "project", "project or global")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	scope := mem.Scope(*scopeText)
	if scope != mem.ScopeProject && scope != mem.ScopeGlobal {
		return nil, nil, errors.New("memory scope is invalid")
	}
	dir := *project
	if scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return nil, nil, errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, scope)
	if err != nil {
		return nil, nil, err
	}
	return scopeText, store, nil
}

func runMemoryHistory(args []string) error {
	if len(args) == 0 {
		return errors.New("memory history requires memory id")
	}
	memoryID := args[0]
	scopeText, store, err := openMemoryScope(args[1:], "memory history")
	if err != nil {
		return err
	}
	keys, err := store.List(context.Background(), mem.FactKindMemoryRevision)
	if err != nil {
		return err
	}
	entries := []memoryRevisionHistoryEntry{}
	for _, key := range keys {
		if !strings.HasPrefix(key, memoryID+"/") {
			continue
		}
		b, err := store.Get(context.Background(), mem.FactKindMemoryRevision, key)
		if err != nil {
			return err
		}
		rev, err := mem.DecodeStrict[mem.MemoryRevision](b)
		if err != nil || rev.MemoryID != memoryID {
			return errors.New("memory revision is invalid")
		}
		entries = append(entries, memoryRevisionHistoryEntry{rev.MemoryID, rev.Revision, rev.MemoryType, rev.CanonicalKey, rev.UsagePolicy, rev.ContentSHA256, rev.CreatedAt})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Revision < entries[j].Revision })
	return writeJSONOutput(struct {
		Scope     mem.Scope                    `json:"scope"`
		MemoryID  string                       `json:"memory_id"`
		Revisions []memoryRevisionHistoryEntry `json:"revisions"`
	}{mem.Scope(*scopeText), memoryID, entries})
}

type memoryUsageHistoryEntry struct {
	UsageID    string `json:"usage_id"`
	Revision   int    `json:"revision"`
	UsageStage string `json:"usage_stage"`
	EpisodeID  string `json:"episode_id,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

func runMemoryUsageHistory(args []string) error {
	if len(args) == 0 {
		return errors.New("memory usage requires memory id")
	}
	memoryID := args[0]
	scopeText, store, err := openMemoryScope(args[1:], "memory usage")
	if err != nil {
		return err
	}
	keys, err := store.List(context.Background(), mem.FactKindMemoryUsage)
	if err != nil {
		return err
	}
	entries := []memoryUsageHistoryEntry{}
	for _, key := range keys {
		b, err := store.Get(context.Background(), mem.FactKindMemoryUsage, key)
		if err != nil {
			return err
		}
		u, err := mem.DecodeStrict[mem.MemoryUsage](b)
		if err != nil {
			return errors.New("memory usage is invalid")
		}
		if u.MemoryID == memoryID {
			entries = append(entries, memoryUsageHistoryEntry{u.UsageID, u.Revision, u.UsageStage, u.EpisodeID, u.OccurredAt})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].OccurredAt < entries[j].OccurredAt })
	return writeJSONOutput(struct {
		Scope    mem.Scope                 `json:"scope"`
		MemoryID string                    `json:"memory_id"`
		Usages   []memoryUsageHistoryEntry `json:"usages"`
	}{mem.Scope(*scopeText), memoryID, entries})
}

func runMemoryStatus(args []string) error {
	fs := flag.NewFlagSet("memory status", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	memoryID := fs.String("memory-id", "", "memory id")
	revision := fs.Int("revision", 0, "memory revision")
	nowText := fs.String("now", "", "explicit evaluation timestamp (RFC3339)")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *memoryID == "" {
		return errors.New("memory-id is required")
	}
	if *nowText == "" {
		return errors.New("now is required for deterministic memory status")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil {
		return errors.New("now must be a valid RFC3339 timestamp")
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	result, err := mem.DeriveState(context.Background(), store, mem.DerivedStateRequest{Scope: sc, Revision: *revision, Now: now.UTC()})
	if err != nil {
		return err
	}
	for _, state := range result.States {
		if state.MemoryID == *memoryID && (*revision == 0 || state.Revision == *revision) {
			return writeJSONOutput(state)
		}
	}
	return errors.New("memory status not found")
}

func runMemoryConsistencyDoctor(args []string) error {
	fs := flag.NewFlagSet("memory doctor", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	report, err := mem.CheckConsistency(context.Background(), store, mem.ConsistencyRequest{Scope: sc})
	if err != nil {
		return err
	}
	return writeJSONOutput(report)
}

func runMemoryOutcomeOverride(args []string) error {
	args = normalizeLeadingTargetArgs(args)
	fs := flag.NewFlagSet("memory outcome override", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	outcomeID := fs.String("outcome-id", "", "outcome id")
	previous := fs.String("previous-effect", "", "current effect")
	newEffect := fs.String("new-effect", "", "corrected effect")
	reason := fs.String("reason", "", "override reason")
	nowText := fs.String("now", "", "explicit evaluation timestamp (RFC3339)")
	sourceType := fs.String("source", "local_user", "source type")
	sourceID := fs.String("source-id", "local_user", "source id")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outcomeID == "" && len(fs.Args()) > 0 {
		*outcomeID = fs.Args()[0]
	}
	if *outcomeID == "" || *previous == "" || *newEffect == "" || *reason == "" {
		return errors.New("outcome-id, previous-effect, new-effect and reason are required")
	}
	if *nowText == "" {
		return errors.New("now is required for deterministic outcome override")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil {
		return errors.New("now must be a valid RFC3339 timestamp")
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	result, err := mem.CommitAttributionOverride(context.Background(), mem.AttributionOverrideRequest{Store: store, OutcomeID: *outcomeID, PreviousEffect: *previous, NewEffect: *newEffect, Reason: *reason, SourceType: *sourceType, SourceID: *sourceID, Now: now.UTC()})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryGovernance(operation string, args []string) error {
	fs := flag.NewFlagSet("memory "+operation, flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	memoryID := fs.String("memory-id", "", "memory id")
	revision := fs.Int("revision", 0, "memory revision")
	reason := fs.String("reason", "", "governance reason")
	source := fs.String("source", "local_user", "governance source")
	nowText := fs.String("now", "", "explicit evaluation timestamp (RFC3339)")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *memoryID == "" || *revision < 1 || *reason == "" {
		return errors.New("memory-id, revision and reason are required")
	}
	if *nowText == "" {
		return errors.New("now is required for deterministic governance")
	}
	now, err := time.Parse(time.RFC3339Nano, *nowText)
	if err != nil {
		return errors.New("now must be a valid RFC3339 timestamp")
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	b, err := store.Get(context.Background(), mem.FactKindMemoryRevision, fmt.Sprintf("%s/%d", *memoryID, *revision))
	if err != nil {
		return err
	}
	rev, err := mem.DecodeStrict[mem.MemoryRevision](b)
	if err != nil {
		return errors.New("memory revision is invalid")
	}
	target := mem.MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	governanceOperation := operation
	if governanceOperation == "freeze" {
		governanceOperation = "manual_freeze"
	}
	request := mem.GovernanceRequest{Store: store, Target: target, Operation: governanceOperation, Reason: *reason, Source: *source, Now: now.UTC()}
	if operation == "unfreeze" {
		request.BasisRefs = []mem.BasisRef{{MemoryRef: &target}}
	}
	result, err := mem.CommitGovernanceEvent(context.Background(), request)
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryGet(args []string) error {
	fs := flag.NewFlagSet("memory get", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	memoryID := fs.String("memory-id", "", "memory id")
	revision := fs.Int("revision", 0, "memory revision")
	includeFrozen := fs.Bool("include-frozen", false, "include frozen memory")
	reviewMode := fs.Bool("review-mode", false, "explicit review mode")
	nowText := fs.String("now", "", "explicit evaluation timestamp (RFC3339; required for review mode)")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *memoryID == "" || *revision < 1 {
		return errors.New("memory-id and revision are required")
	}
	if *reviewMode && *nowText == "" {
		return errors.New("now is required for deterministic review read")
	}
	now := time.Time{}
	if *reviewMode {
		var err error
		now, err = time.Parse(time.RFC3339Nano, *nowText)
		if err != nil {
			return errors.New("now must be a valid RFC3339 timestamp")
		}
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	if err != nil {
		return err
	}
	b, err := store.Get(context.Background(), mem.FactKindMemoryRevision, fmt.Sprintf("%s/%d", *memoryID, *revision))
	if err != nil {
		return err
	}
	rev, err := mem.DecodeStrict[mem.MemoryRevision](b)
	if err != nil {
		return errors.New("memory revision is invalid")
	}
	got, err := mem.ReadMemoryForReview(context.Background(), mem.ReviewReadRequest{Store: store, Target: mem.MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}, IncludeFrozen: *includeFrozen, ReviewMode: *reviewMode, Now: now.UTC()})
	if err != nil {
		return err
	}
	return writeJSONOutput(got)
}

func runMemoryOutcomeCapture(args []string) error {
	fs := flag.NewFlagSet("memory outcome capture", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	receiptFile := fs.String("attribution-receipt", "", "Attribution receipt JSON")
	external := fs.Bool("external-failure", false, "task failed for an externally verified reason")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *receiptFile == "" {
		return errors.New("attribution-receipt is required")
	}
	b, err := readBoundedJSONFile(*receiptFile)
	if err != nil {
		return err
	}
	receipt, err := mem.DecodeStrict[mem.AttributionReceipt](b)
	if err != nil {
		return errors.New("attribution receipt is invalid")
	}
	dir := *project
	if receipt.EpisodeRef.Scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, receipt.EpisodeRef.Scope)
	if err != nil {
		return err
	}
	result, err := mem.CommitOutcomes(context.Background(), mem.AttributionRequest{Store: store, Receipt: receipt, ExternalFailure: *external})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func runMemoryUsageCapture(args []string) error {
	fs := flag.NewFlagSet("memory usage capture", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	librarianFile := fs.String("librarian-receipt", "", "validated Librarian receipt JSON")
	usageFile := fs.String("usage-receipt", "", "MemoryUsage receipt JSON")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *librarianFile == "" || *usageFile == "" {
		return errors.New("librarian-receipt and usage-receipt are required")
	}
	lb, err := readBoundedJSONFile(*librarianFile)
	if err != nil {
		return err
	}
	var librarian mem.LibrarianReceipt
	if err := strictJSON(lb, &librarian); err != nil {
		return errors.New("librarian receipt is invalid")
	}
	ub, err := readBoundedJSONFile(*usageFile)
	if err != nil {
		return err
	}
	usage, err := mem.DecodeStrict[mem.MemoryUsageReceipt](ub)
	if err != nil {
		return errors.New("usage receipt is invalid")
	}
	dir := *project
	if usage.EpisodeRef.Scope == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, usage.EpisodeRef.Scope)
	if err != nil {
		return err
	}
	result, err := mem.CommitMemoryUsages(context.Background(), mem.CaptureUsageRequest{Store: store, LibrarianReceipt: librarian, UsageReceipt: usage})
	if err != nil {
		return err
	}
	return writeJSONOutput(result)
}

func memoryStoreRoot(project string) string {
	return filepath.Join(project, ".reasonix", "omr", "memory")
}
func openExistingMemoryStore(project string, scope mem.Scope) (*mem.FactStore, error) {
	root := memoryStoreRoot(project)
	for _, p := range []string{root, filepath.Join(root, "facts"), filepath.Join(root, "locks"), filepath.Join(root, "diagnostics")} {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("memory store is unavailable")
		}
	}
	if scope == mem.ScopeGlobal {
		return mem.OpenGlobal(root, mem.Options{})
	}
	return mem.OpenProject(root, mem.Options{})
}

func runMemoryContext(args []string) error {
	fs := flag.NewFlagSet("memory episodic context", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	projectID := fs.String("project-scope-id", "project", "project scope id")
	globalID := fs.String("global-scope-id", "global", "global scope id")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	doc := episodicContextDocument{SchemaVersion: 1}
	s, err := openExistingMemoryStore(*project, mem.ScopeProject)
	if err != nil {
		return err
	}
	doc.Project, err = mem.PinCurrentEpisodicContext(context.Background(), s, *projectID)
	if err != nil {
		return err
	}
	if *global != "" {
		g, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
		if err != nil {
			return err
		}
		doc.Global, err = mem.PinCurrentEpisodicContext(context.Background(), g, *globalID)
		if err != nil {
			return err
		}
	}
	return writeJSONOutput(doc)
}

func episodicFlags(name string, args []string) (*episodicContextDocument, string, *mem.FactStore, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	contextFile := fs.String("context-file", "", "fixed context JSON")
	scope := fs.String("scope", "project", "project or global")
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, "", nil, err
	}
	if *contextFile == "" {
		return nil, "", nil, errors.New("context-file is required")
	}
	doc, err := readEpisodicContext(*contextFile)
	if err != nil {
		return nil, "", nil, err
	}
	sc := mem.Scope(*scope)
	if sc != mem.ScopeProject && sc != mem.ScopeGlobal {
		return nil, "", nil, errors.New("memory scope is invalid")
	}
	dir := *project
	if sc == mem.ScopeGlobal {
		dir = *global
	}
	if dir == "" {
		return nil, "", nil, errors.New("memory scope directory is unavailable")
	}
	store, err := openExistingMemoryStore(dir, sc)
	return doc, *scope, store, err
}

func pinnedFrom(doc *episodicContextDocument, scope string) (mem.EpisodicScopeContext, error) {
	if scope == "project" && doc.Project != nil {
		return *doc.Project, nil
	}
	if scope == "global" && doc.Global != nil {
		return *doc.Global, nil
	}
	return mem.EpisodicScopeContext{}, errors.New("fixed episodic scope is unavailable")
}
func runMemoryIndex(args []string) error {
	doc, scope, s, err := episodicFlags("memory episodic index", args)
	if err != nil {
		return err
	}
	p, err := pinnedFrom(doc, scope)
	if err != nil {
		return err
	}
	v, err := mem.ReadEpisodicIndex(context.Background(), s, p)
	if err != nil {
		return err
	}
	return writeJSONOutput(v)
}
func runMemoryCard(args []string) error {
	episodeID, args, err := extractStringFlag(args, "episode-id")
	if err != nil || episodeID == "" {
		return errors.New("episode-id is required")
	}
	doc, scope, s, err := episodicFlags("memory episodic card", args)
	if err != nil {
		return err
	}
	p, err := pinnedFrom(doc, scope)
	if err != nil {
		return err
	}
	idx, err := mem.ReadEpisodicIndex(context.Background(), s, p)
	if err != nil {
		return err
	}
	for _, entry := range idx.Entries {
		if entry.EpisodeRef.EpisodeID == episodeID {
			v, err := mem.ReadEpisodeCard(context.Background(), s, p, entry.EpisodeRef)
			if err != nil {
				return err
			}
			return writeJSONOutput(v)
		}
	}
	return errors.New("episode is not present in fixed index")
}

func runMemoryEpisodeList(args []string) error {
	doc, scope, store, err := episodicFlags("memory episode list", args)
	if err != nil {
		return err
	}
	pinned, err := pinnedFrom(doc, scope)
	if err != nil {
		return err
	}
	index, err := mem.ReadEpisodicIndex(context.Background(), store, pinned)
	if err != nil {
		return err
	}
	return writeJSONOutput(index)
}

func runMemoryEpisodeShow(args []string) error {
	episodeID, args, err := extractStringFlag(args, "episode-id")
	if err != nil || episodeID == "" {
		return errors.New("episode-id is required")
	}
	doc, scope, store, err := episodicFlags("memory episode show", args)
	if err != nil {
		return err
	}
	pinned, err := pinnedFrom(doc, scope)
	if err != nil {
		return err
	}
	index, err := mem.ReadEpisodicIndex(context.Background(), store, pinned)
	if err != nil {
		return err
	}
	for _, entry := range index.Entries {
		if entry.EpisodeRef.EpisodeID == episodeID {
			card, err := mem.ReadEpisodeCard(context.Background(), store, pinned, entry.EpisodeRef)
			if err != nil {
				return err
			}
			return writeJSONOutput(card)
		}
	}
	return errors.New("episode is not present in fixed index")
}
func runMemoryDoctor(args []string) error {
	doc, scope, s, err := episodicFlags("memory episodic doctor", args)
	if err != nil {
		return err
	}
	p, err := pinnedFrom(doc, scope)
	if err != nil {
		return err
	}
	v, err := mem.CheckEpisodicGeneration(context.Background(), s, p)
	if err != nil {
		return err
	}
	return writeJSONOutput(v)
}
func runMemoryReceipt(args []string) error {
	fs := flag.NewFlagSet("memory episodic validate-receipt", flag.ContinueOnError)
	contextFile := fs.String("context-file", "", "fixed context JSON")
	receiptFile := fs.String("receipt-file", "", "episodic receipt JSON")
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *contextFile == "" || *receiptFile == "" {
		return errors.New("context-file and receipt-file are required")
	}
	doc, err := readEpisodicContext(*contextFile)
	if err != nil {
		return err
	}
	b, err := readBoundedJSONFile(*receiptFile)
	if err != nil {
		return err
	}
	var receipt mem.EpisodicRecallReceipt
	if err := strictJSON(b, &receipt); err != nil {
		return errors.New("episodic receipt is invalid")
	}
	stores := make(map[mem.Scope]*mem.FactStore, 2)
	contexts := make(map[mem.Scope]mem.EpisodicScopeContext, 2)
	if doc.Project != nil {
		s, err := openExistingMemoryStore(*project, mem.ScopeProject)
		if err != nil {
			return err
		}
		stores[mem.ScopeProject], contexts[mem.ScopeProject] = s, *doc.Project
	}
	if doc.Global != nil {
		if *global == "" {
			return errors.New("global memory directory is unavailable")
		}
		s, err := openExistingMemoryStore(*global, mem.ScopeGlobal)
		if err != nil {
			return err
		}
		stores[mem.ScopeGlobal], contexts[mem.ScopeGlobal] = s, *doc.Global
	}
	if err := mem.ValidateEpisodicReceipt(context.Background(), stores, contexts, receipt); err != nil {
		return err
	}
	return writeJSONOutput(map[string]any{"valid": true, "schema_version": 1})
}

func readEpisodicContext(path string) (*episodicContextDocument, error) {
	b, err := readBoundedJSONFile(path)
	if err != nil {
		return nil, err
	}
	var doc episodicContextDocument
	if err := strictJSON(b, &doc); err != nil || doc.SchemaVersion != 1 || (doc.Project == nil && doc.Global == nil) {
		return nil, errors.New("episodic context is invalid")
	}
	return &doc, nil
}

func readBoundedJSONFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("JSON file is required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return nil, errors.New("JSON file is unsafe")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("JSON file is unreadable")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("JSON file is unreadable")
	}
	return b, nil
}
func strictJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func extractStringFlag(args []string, name string) (string, []string, error) {
	want := "--" + name
	for i, arg := range args {
		if arg == want {
			if i+1 >= len(args) {
				return "", args, errors.New("flag value is required")
			}
			return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...), nil
		}
		prefix := want + "="
		if len(arg) >= len(prefix) && arg[:len(prefix)] == prefix {
			return arg[len(prefix):], append(append([]string{}, args[:i]...), args[i+1:]...), nil
		}
	}
	return "", args, nil
}
