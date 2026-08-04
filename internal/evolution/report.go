package evolution

import (
	"sort"
	"time"
)

// ObservationTarget is the default number of related episodes needed for an
// observed decision. It is deliberately an evidence-count heuristic, not a
// model-quality claim.
const ObservationTarget = 5

// Report is a deterministic, aggregate-only view of Evolution history. It
// deliberately excludes prompts, command text, model reasoning, session ids,
// and any directional "improvement" conclusion.
type Report struct {
	SchemaVersion  int                           `json:"schema_version"`
	ScopeID        string                        `json:"scope_id"`
	Episodes       int                           `json:"episodes"`
	Successes      int                           `json:"successes"`
	Failures       int                           `json:"failures"`
	SuccessRate    float64                       `json:"success_rate"`
	PromptTokens   int                           `json:"prompt_tokens"`
	OutputTokens   int                           `json:"output_tokens"`
	DurationMs     int64                         `json:"duration_ms"`
	FailureClasses map[string]int                `json:"failure_classes,omitempty"`
	TaskClasses    []TaskClassStats              `json:"task_classes,omitempty"`
	Proposals      int                           `json:"proposals"`
	Approved       int                           `json:"approved"`
	RolledBack     int                           `json:"rolled_back"`
	Pending        int                           `json:"pending"`
	Overlay        bool                          `json:"overlay"`
	ProposalScores map[string]int                `json:"proposal_scores,omitempty"`
	ProposalStats  []ProposalStats               `json:"proposal_stats,omitempty"`
	Observations   map[string]ObservationSummary `json:"observations,omitempty"`
}

// TaskClassStats aggregates episodes by TaskClass.
type TaskClassStats struct {
	TaskClass    string  `json:"task_class"`
	Episodes     int     `json:"episodes"`
	Successes    int     `json:"successes"`
	Failures     int     `json:"failures"`
	SuccessRate  float64 `json:"success_rate"`
	FailureRate  float64 `json:"failure_rate"`
	PromptTokens int     `json:"prompt_tokens"`
	OutputTokens int     `json:"output_tokens"`
	DurationMs   int64   `json:"duration_ms"`
}

// ProposalStats aggregates before/after observations for one proposal without
// any quality or causality claims.
type ProposalStats struct {
	ProposalID          string  `json:"proposal_id"`
	Status              string  `json:"status"`
	Before              int     `json:"before"`
	After               int     `json:"after"`
	AfterSuccesses      int     `json:"after_successes"`
	AfterFailures       int     `json:"after_failures"`
	AfterSuccessRate    float64 `json:"after_success_rate"`
	AfterFailureRate    float64 `json:"after_failure_rate"`
	AfterPromptTokens   int     `json:"after_prompt_tokens"`
	AfterOutputTokens   int     `json:"after_output_tokens"`
	AfterDurationMs     int64   `json:"after_duration_ms"`
	ObservationTarget   int     `json:"observation_target"`
	ObservationProgress int     `json:"observation_progress"`
	RollbackReason      string  `json:"rollback_reason,omitempty"`
}

// ObservationSummary is kept for backward compatibility with the v2.0.2
// per-proposal observation summary.
type ObservationSummary struct {
	Before         int    `json:"before"`
	After          int    `json:"after"`
	AfterSuccesses int    `json:"after_successes"`
	AfterFailures  int    `json:"after_failures"`
	Status         string `json:"status"`
}

// durationRangeMs returns the span between the earliest and latest CreatedAt
// timestamps in milliseconds. It returns 0 when fewer than two timestamps
// are present or when they cannot be parsed.
func durationRangeMs(times []string) int64 {
	var ts []time.Time
	for _, raw := range times {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			continue
		}
		ts = append(ts, t)
	}
	if len(ts) < 2 {
		return 0
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	return ts[len(ts)-1].Sub(ts[0]).Milliseconds()
}

// BuildReport computes the deterministic aggregate report for one store.
func BuildReport(store Store) (Report, error) {
	episodes, err := store.ListEpisodes()
	if err != nil {
		return Report{}, err
	}
	proposals, err := store.ListProposals()
	if err != nil {
		return Report{}, err
	}
	report := Report{
		SchemaVersion:  SchemaVersion,
		ScopeID:        store.ScopeID,
		Episodes:       len(episodes),
		FailureClasses: map[string]int{},
		Proposals:      len(proposals),
		ProposalScores: map[string]int{},
		Observations:   map[string]ObservationSummary{},
	}
	var episodeTimes []string
	for _, episode := range episodes {
		report.PromptTokens += episode.PromptTokens
		report.OutputTokens += episode.OutputTokens
		episodeTimes = append(episodeTimes, episode.CreatedAt)
		if episode.Succeeded {
			report.Successes++
		} else {
			report.Failures++
			report.FailureClasses[episode.FailureClass]++
		}
	}
	report.DurationMs = durationRangeMs(episodeTimes)
	if report.Episodes > 0 {
		report.SuccessRate = float64(report.Successes) / float64(report.Episodes)
	}

	// Task-class aggregation with stable ordering.
	taskAgg := map[string]*TaskClassStats{}
	taskTimes := map[string][]string{}
	var taskOrder []string
	for _, episode := range episodes {
		tc := taskAgg[episode.TaskClass]
		if tc == nil {
			tc = &TaskClassStats{TaskClass: episode.TaskClass}
			taskAgg[episode.TaskClass] = tc
			taskOrder = append(taskOrder, episode.TaskClass)
		}
		tc.Episodes++
		tc.PromptTokens += episode.PromptTokens
		tc.OutputTokens += episode.OutputTokens
		taskTimes[episode.TaskClass] = append(taskTimes[episode.TaskClass], episode.CreatedAt)
		if episode.Succeeded {
			tc.Successes++
		} else {
			tc.Failures++
		}
	}
	for _, name := range taskOrder {
		tc := taskAgg[name]
		if tc.Episodes > 0 {
			tc.SuccessRate = float64(tc.Successes) / float64(tc.Episodes)
			tc.FailureRate = float64(tc.Failures) / float64(tc.Episodes)
		}
		tc.DurationMs = durationRangeMs(taskTimes[name])
		report.TaskClasses = append(report.TaskClasses, *tc)
	}
	sort.Slice(report.TaskClasses, func(i, j int) bool { return report.TaskClasses[i].TaskClass < report.TaskClasses[j].TaskClass })

	// Per-proposal scoring and status rollup.
	for _, proposal := range proposals {
		report.ProposalScores[proposal.ID] = AssessProposal(proposal).Score
		switch proposal.Status {
		case "approved":
			report.Approved++
		case "rolled_back":
			report.RolledBack++
		case "pending":
			report.Pending++
		}
	}

	// Observation aggregation.
	observations, err := store.ListObservations()
	if err != nil {
		return Report{}, err
	}
	type obsAcc struct {
		before     int
		after      int
		afterOK    int
		afterFail  int
		afterP     int
		afterO     int
		afterTimes []string
	}
	acc := map[string]*obsAcc{}
	for _, observation := range observations {
		a := acc[observation.ProposalID]
		if a == nil {
			a = &obsAcc{}
			acc[observation.ProposalID] = a
		}
		if observation.Phase == "before" {
			a.before++
			continue
		}
		a.after++
		a.afterP += observation.PromptTokens
		a.afterO += observation.OutputTokens
		a.afterTimes = append(a.afterTimes, observation.CreatedAt)
		if observation.Succeeded {
			a.afterOK++
		} else {
			a.afterFail++
		}
	}
	proposalByID := map[string]Proposal{}
	for _, proposal := range proposals {
		proposalByID[proposal.ID] = proposal
	}
	for _, proposal := range proposals {
		a := acc[proposal.ID]
		ps := ProposalStats{
			ProposalID:        proposal.ID,
			Status:            proposal.Status,
			ObservationTarget: ObservationTarget,
		}
		if a != nil {
			ps.Before = a.before
			ps.After = a.after
			ps.AfterSuccesses = a.afterOK
			ps.AfterFailures = a.afterFail
			ps.AfterPromptTokens = a.afterP
			ps.AfterOutputTokens = a.afterO
			ps.AfterDurationMs = durationRangeMs(a.afterTimes)
			ps.ObservationProgress = a.after
			if a.after > 0 {
				ps.AfterSuccessRate = float64(a.afterOK) / float64(a.after)
				ps.AfterFailureRate = float64(a.afterFail) / float64(a.after)
			}
		}
		switch {
		case proposal.Status == "rolled_back":
			ps.Status = "rolled_back"
			ps.RollbackReason = proposal.RollbackReason
		case ps.After < ObservationTarget:
			ps.Status = "insufficient_evidence"
		default:
			ps.Status = "observed"
		}
		report.ProposalStats = append(report.ProposalStats, ps)

		summary := report.Observations[proposal.ID]
		if a != nil {
			summary.Before = a.before
			summary.After = a.after
			summary.AfterSuccesses = a.afterOK
			summary.AfterFailures = a.afterFail
		}
		summary.Status = ps.Status
		report.Observations[proposal.ID] = summary
	}
	// Backward-compatible status for observations whose proposal no longer
	// exists in the proposals collection.
	for proposalID := range acc {
		if _, ok := proposalByID[proposalID]; ok {
			continue
		}
		a := acc[proposalID]
		summary := report.Observations[proposalID]
		summary.Before = a.before
		summary.After = a.after
		summary.AfterSuccesses = a.afterOK
		summary.AfterFailures = a.afterFail
		if a.after < ObservationTarget {
			summary.Status = "insufficient_evidence"
		} else {
			summary.Status = "observed"
		}
		report.Observations[proposalID] = summary
	}
	sort.Slice(report.ProposalStats, func(i, j int) bool {
		return report.ProposalStats[i].ProposalID < report.ProposalStats[j].ProposalID
	})
	if _, err := store.ReadOverlay(); err == nil {
		report.Overlay = true
	}
	return report, nil
}

// HistoryDetail is the detailed per-proposal observation history. It contains
// only aggregate episode summaries plus observation identifiers — never raw
// session ids, prompts, or model output.
type HistoryDetail struct {
	SchemaVersion       int              `json:"schema_version"`
	ScopeID             string           `json:"scope_id"`
	ProposalID          string           `json:"proposal_id"`
	Status              string           `json:"status"`
	Before              int              `json:"before"`
	After               int              `json:"after"`
	AfterSuccesses      int              `json:"after_successes"`
	AfterFailures       int              `json:"after_failures"`
	ObservationProgress int              `json:"observation_progress"`
	ObservationTarget   int              `json:"observation_target"`
	RollbackReason      string           `json:"rollback_reason,omitempty"`
	Observations        []Observation    `json:"observations"`
	BeforeEpisodes      []EpisodeSummary `json:"before_episodes"`
	AfterEpisodes       []EpisodeSummary `json:"after_episodes"`
}

// EpisodeSummary is a sanitized episode view for history output.
type EpisodeSummary struct {
	ID           string `json:"id"`
	TaskClass    string `json:"task_class"`
	FailureClass string `json:"failure_class,omitempty"`
	Succeeded    bool   `json:"succeeded"`
	ExitCode     int    `json:"exit_code"`
	PromptTokens int    `json:"prompt_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// BuildHistory returns the detailed observation history for one proposal.
func BuildHistory(store Store, proposalID string) (HistoryDetail, error) {
	proposal, err := store.LoadProposal(proposalID)
	if err != nil {
		return HistoryDetail{}, err
	}
	observations, err := store.ListObservations()
	if err != nil {
		return HistoryDetail{}, err
	}
	episodeByID := map[string]Episode{}
	episodes, err := store.ListEpisodes()
	if err != nil {
		return HistoryDetail{}, err
	}
	for _, e := range episodes {
		episodeByID[e.ID] = e
	}
	detail := HistoryDetail{
		SchemaVersion:     SchemaVersion,
		ScopeID:           store.ScopeID,
		ProposalID:        proposal.ID,
		Status:            proposal.Status,
		RollbackReason:    proposal.RollbackReason,
		ObservationTarget: ObservationTarget,
	}
	var afterTimes []string
	for _, o := range observations {
		if o.ProposalID != proposal.ID {
			continue
		}
		detail.Observations = append(detail.Observations, o)
		if o.Phase == "before" {
			detail.Before++
			if e, ok := episodeByID[o.EpisodeID]; ok {
				detail.BeforeEpisodes = append(detail.BeforeEpisodes, summarize(e))
			}
			continue
		}
		detail.After++
		afterTimes = append(afterTimes, o.CreatedAt)
		if o.Succeeded {
			detail.AfterSuccesses++
		} else {
			detail.AfterFailures++
		}
		if e, ok := episodeByID[o.EpisodeID]; ok {
			detail.AfterEpisodes = append(detail.AfterEpisodes, summarize(e))
		}
	}
	detail.ObservationProgress = detail.After
	switch {
	case proposal.Status == "rolled_back":
		detail.Status = "rolled_back"
	case detail.After < ObservationTarget:
		detail.Status = "insufficient_evidence"
	default:
		detail.Status = "observed"
	}
	// Deterministic ordering.
	sort.Slice(detail.Observations, func(i, j int) bool {
		return detail.Observations[i].ID < detail.Observations[j].ID
	})
	sort.Slice(detail.BeforeEpisodes, func(i, j int) bool {
		return detail.BeforeEpisodes[i].CreatedAt < detail.BeforeEpisodes[j].CreatedAt
	})
	sort.Slice(detail.AfterEpisodes, func(i, j int) bool {
		return detail.AfterEpisodes[i].CreatedAt < detail.AfterEpisodes[j].CreatedAt
	})
	return detail, nil
}

func summarize(e Episode) EpisodeSummary {
	return EpisodeSummary{
		ID:           e.ID,
		TaskClass:    e.TaskClass,
		FailureClass: e.FailureClass,
		Succeeded:    e.Succeeded,
		ExitCode:     e.ExitCode,
		PromptTokens: e.PromptTokens,
		OutputTokens: e.OutputTokens,
		CreatedAt:    e.CreatedAt,
	}
}
