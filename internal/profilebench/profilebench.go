// Package profilebench benchmarks engineering-process outcomes of OMR
// profiles/prompts against offline, deterministic fixtures. It measures
// governance indicators (acceptance pass rate, evidence completeness,
// out-of-scope changes, human corrections, tokens, cost, duration) and
// explicitly does NOT claim model-quality proof. No API key or network
// provider is needed: replay outcomes come from the fixtures themselves.
package profilebench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NonQualityClaim is the explicit disclaimer carried by every report.
const NonQualityClaim = "process metrics only; not a model quality proof"

// ProfileFixture describes one offline benchmark case for a profile.
type ProfileFixture struct {
	SchemaVersion    int            `json:"schema_version"`
	ID               string         `json:"id"`
	Profile          string         `json:"profile"`
	Task             string         `json:"task"`
	Acceptance       []string       `json:"acceptance"`
	RequiredEvidence []string       `json:"required_evidence,omitempty"`
	ForbiddenPaths   []string       `json:"forbidden_paths,omitempty"`
	Replay           *ProfileReplay `json:"replay,omitempty"`
	NativeReplay     *ProfileReplay `json:"native_replay,omitempty"`
	OMRReplay        *ProfileReplay `json:"omr_replay,omitempty"`
}

// ProfileReplay is the deterministic outcome of one fixture run. It never
// contains prompts, command bodies, or model reasoning.
type ProfileReplay struct {
	AcceptanceMet     []bool         `json:"acceptance_met"`
	EvidenceCovered   []bool         `json:"evidence_covered,omitempty"`
	ChangedPaths      []string       `json:"changed_paths,omitempty"`
	OutOfScopeChanges int            `json:"out_of_scope_changes,omitempty"`
	HumanCorrections  int            `json:"human_corrections,omitempty"`
	Metrics           ProfileMetrics `json:"metrics,omitempty"`
}

// ProfileMetrics aggregates sanitized token/cost/duration numbers.
type ProfileMetrics struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
	DurationMs       int64   `json:"duration_ms"`
}

// Evaluation is the per-fixture process assessment.
type Evaluation struct {
	FixtureID            string         `json:"fixture_id"`
	Profile              string         `json:"profile"`
	AcceptancePassRate   float64        `json:"acceptance_pass_rate"`
	EvidenceCompleteRate float64        `json:"evidence_complete_rate"`
	OutOfScopeChanges    int            `json:"out_of_scope_changes"`
	HumanCorrections     int            `json:"human_corrections"`
	Metrics              ProfileMetrics `json:"metrics"`
	Qualified            bool           `json:"qualified"`
	Failures             []string       `json:"failures,omitempty"`
}

// ProfileReport is the aggregate, deterministic view of a benchmark run.
type ProfileReport struct {
	SchemaVersion  int            `json:"schema_version"`
	RunID          string         `json:"run_id"`
	Profile        string         `json:"profile,omitempty"`
	Matrix         bool           `json:"matrix"`
	FixtureCount   int            `json:"fixture_count"`
	EvaluatedCount int            `json:"evaluated_count"`
	QualifiedCount int            `json:"qualified_count"`
	QualifiedRate  float64        `json:"qualified_rate"`
	Metrics        ProfileMetrics `json:"metrics"`
	Results        []Evaluation   `json:"results"`
	Claim          string         `json:"claim"`
}

// LoadFixture parses and validates one profile-quality fixture file.
// Unknown fields are rejected so schema drift fails closed.
func LoadFixture(path string) (ProfileFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProfileFixture{}, err
	}
	var fixture ProfileFixture
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fixture); err != nil {
		return ProfileFixture{}, fmt.Errorf("parse profile fixture %s: %w", path, err)
	}
	if err := fixture.Validate(); err != nil {
		return ProfileFixture{}, err
	}
	return fixture, nil
}

// Validate checks the invariant fields of a profile fixture.
func (f ProfileFixture) Validate() error {
	if f.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", f.SchemaVersion)
	}
	if f.ID == "" || f.Profile == "" || f.Task == "" || len(f.Acceptance) == 0 {
		return fmt.Errorf("fixture %s requires id, profile, task, and acceptance", f.ID)
	}
	if f.Replay == nil && f.NativeReplay == nil && f.OMRReplay == nil {
		return fmt.Errorf("fixture %s requires replay or native_replay/omr_replay", f.ID)
	}
	return nil
}

// Discover walks a directory tree for fixture.yaml files and returns them in
// deterministic (path-sorted) order.
func Discover(root string) ([]ProfileFixture, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "fixture.yaml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	fixtures := make([]ProfileFixture, 0, len(paths))
	for _, path := range paths {
		fixture, err := LoadFixture(path)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

// Evaluate computes the process metrics for one fixture against its replay.
func Evaluate(fixture ProfileFixture, replay ProfileReplay) Evaluation {
	ev := Evaluation{
		FixtureID:         fixture.ID,
		Profile:           fixture.Profile,
		OutOfScopeChanges: replay.OutOfScopeChanges,
		HumanCorrections:  replay.HumanCorrections,
		Metrics:           replay.Metrics,
	}
	met := 0
	for i, accepted := range replay.AcceptanceMet {
		if i >= len(fixture.Acceptance) {
			continue
		}
		if accepted {
			met++
		} else {
			ev.Failures = append(ev.Failures, fmt.Sprintf("acceptance #%d not met: %s", i, fixture.Acceptance[i]))
		}
	}
	if len(fixture.Acceptance) > 0 {
		ev.AcceptancePassRate = float64(met) / float64(len(fixture.Acceptance))
	}
	covered := 0
	coveredFalse := 0
	for i, coveredOK := range replay.EvidenceCovered {
		if len(fixture.RequiredEvidence) > 0 && i >= len(fixture.RequiredEvidence) {
			continue
		}
		if coveredOK {
			covered++
		} else {
			coveredFalse++
			if i < len(fixture.RequiredEvidence) {
				ev.Failures = append(ev.Failures, fmt.Sprintf("evidence #%d missing: %s", i, fixture.RequiredEvidence[i]))
			}
		}
	}
	evidenceDenominator := len(fixture.RequiredEvidence)
	if evidenceDenominator == 0 {
		evidenceDenominator = len(replay.EvidenceCovered)
	}
	switch {
	case evidenceDenominator > 0:
		ev.EvidenceCompleteRate = float64(covered) / float64(evidenceDenominator)
	case coveredFalse > 0:
		ev.EvidenceCompleteRate = 0
	default:
		ev.EvidenceCompleteRate = 1 // no evidence requirements, nothing missing
	}
	for _, changed := range replay.ChangedPaths {
		if matchesAny(changed, fixture.ForbiddenPaths) {
			ev.Failures = append(ev.Failures, "modified path outside fixture scope: "+changed)
		}
	}
	if ev.OutOfScopeChanges > 0 {
		ev.Failures = append(ev.Failures, fmt.Sprintf("%d out-of-scope change(s)", ev.OutOfScopeChanges))
	}
	ev.Qualified = len(ev.Failures) == 0
	return ev
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasPrefix(path, p) || strings.Contains(path, p) {
			return true
		}
	}
	return false
}

// EvaluateAll aggregates per-fixture evaluations into one deterministic report.
func EvaluateAll(fixtures []ProfileFixture, results map[string]ProfileReplay, runID string, matrix bool, opts ...Option) ProfileReport {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	report := ProfileReport{
		SchemaVersion: 1,
		RunID:         runID,
		Matrix:        matrix,
		Claim:         NonQualityClaim,
	}
	if o.profile != "" {
		report.Profile = o.profile
	}
	selected := make([]ProfileFixture, 0, len(fixtures))
	for _, fixture := range fixtures {
		if o.profile != "" && fixture.Profile != o.profile {
			continue
		}
		selected = append(selected, fixture)
	}
	report.FixtureCount = len(selected)
	for _, fixture := range selected {
		replay, ok := results[fixture.ID]
		if !ok {
			continue
		}
		report.EvaluatedCount++
		ev := Evaluate(fixture, replay)
		report.Results = append(report.Results, ev)
		if ev.Qualified {
			report.QualifiedCount++
		}
		report.Metrics.PromptTokens += ev.Metrics.PromptTokens
		report.Metrics.CompletionTokens += ev.Metrics.CompletionTokens
		report.Metrics.Cost += ev.Metrics.Cost
		report.Metrics.DurationMs += ev.Metrics.DurationMs
	}
	if report.EvaluatedCount > 0 {
		report.QualifiedRate = float64(report.QualifiedCount) / float64(report.EvaluatedCount)
	} else {
		report.QualifiedRate = 1
	}
	sort.Slice(report.Results, func(i, j int) bool {
		if report.Results[i].Profile != report.Results[j].Profile {
			return report.Results[i].Profile < report.Results[j].Profile
		}
		return report.Results[i].FixtureID < report.Results[j].FixtureID
	})
	return report
}

// ReplayPaired returns the native and OMR evaluations of one paired fixture.
func ReplayPaired(fixture ProfileFixture) (Evaluation, Evaluation, error) {
	if fixture.NativeReplay == nil || fixture.OMRReplay == nil {
		return Evaluation{}, Evaluation{}, fmt.Errorf("fixture %s requires both native_replay and omr_replay", fixture.ID)
	}
	return Evaluate(fixture, *fixture.NativeReplay), Evaluate(fixture, *fixture.OMRReplay), nil
}

type options struct {
	profile string
}

// Option configures EvaluateAll.
type Option func(*options)

// WithProfile restricts the report to one profile.
func WithProfile(profile string) Option {
	return func(o *options) { o.profile = profile }
}
