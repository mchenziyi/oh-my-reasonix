package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if len(args) < 2 {
		return errors.New("memory requires an episodic, usage or outcome command")
	}
	if args[0] == "usage" {
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
	switch args[0] {
	case "pin", "unpin", "freeze", "unfreeze", "archive":
		return runMemoryGovernance(args[0], args[1:])
	}
	if args[0] != "episodic" {
		return errors.New("memory requires get, episodic, usage or outcome command")
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

func runMemoryOutcomeOverride(args []string) error {
	fs := flag.NewFlagSet("memory outcome override", flag.ContinueOnError)
	project := fs.String("project-dir", ".", "project directory")
	global := fs.String("global-dir", "", "global memory directory")
	scope := fs.String("scope", "project", "project or global")
	outcomeID := fs.String("outcome-id", "", "outcome id")
	previous := fs.String("previous-effect", "", "current effect")
	newEffect := fs.String("new-effect", "", "corrected effect")
	reason := fs.String("reason", "", "override reason")
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
	result, err := mem.CommitAttributionOverride(context.Background(), mem.AttributionOverrideRequest{Store: store, OutcomeID: *outcomeID, PreviousEffect: *previous, NewEffect: *newEffect, Reason: *reason, SourceType: *sourceType, SourceID: *sourceID, Now: time.Now().UTC()})
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
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *memoryID == "" || *revision < 1 || *reason == "" {
		return errors.New("memory-id, revision and reason are required")
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
	request := mem.GovernanceRequest{Store: store, Target: target, Operation: operation, Reason: *reason, Source: *source, Now: time.Now().UTC()}
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
	_ = fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *memoryID == "" || *revision < 1 {
		return errors.New("memory-id and revision are required")
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
	got, err := mem.ReadMemoryForReview(context.Background(), mem.ReviewReadRequest{Store: store, Target: mem.MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}, IncludeFrozen: *includeFrozen, ReviewMode: *reviewMode, Now: time.Now().UTC()})
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
