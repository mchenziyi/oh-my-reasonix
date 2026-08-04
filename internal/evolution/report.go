package evolution

// Report is a deterministic, aggregate-only view of Evolution history. It
// deliberately excludes prompts, command text, and model reasoning.
type Report struct {
	SchemaVersion  int            `json:"schema_version"`
	ScopeID        string         `json:"scope_id"`
	Episodes       int            `json:"episodes"`
	Successes      int            `json:"successes"`
	Failures       int            `json:"failures"`
	SuccessRate    float64        `json:"success_rate"`
	PromptTokens   int            `json:"prompt_tokens"`
	OutputTokens   int            `json:"output_tokens"`
	FailureClasses map[string]int `json:"failure_classes,omitempty"`
	Proposals      int            `json:"proposals"`
	Approved       int            `json:"approved"`
	RolledBack     int            `json:"rolled_back"`
	Pending        int            `json:"pending"`
	Overlay        bool           `json:"overlay"`
	ProposalScores map[string]int `json:"proposal_scores,omitempty"`
}

func BuildReport(store Store) (Report, error) {
	episodes, err := store.ListEpisodes()
	if err != nil {
		return Report{}, err
	}
	proposals, err := store.ListProposals()
	if err != nil {
		return Report{}, err
	}
	report := Report{SchemaVersion: SchemaVersion, ScopeID: store.ScopeID, Episodes: len(episodes), FailureClasses: map[string]int{}, Proposals: len(proposals), ProposalScores: map[string]int{}}
	for _, episode := range episodes {
		report.PromptTokens += episode.PromptTokens
		report.OutputTokens += episode.OutputTokens
		if episode.Succeeded {
			report.Successes++
		} else {
			report.Failures++
			report.FailureClasses[episode.FailureClass]++
		}
	}
	if report.Episodes > 0 {
		report.SuccessRate = float64(report.Successes) / float64(report.Episodes)
	}
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
	if _, err := store.ReadOverlay(); err == nil {
		report.Overlay = true
	}
	return report, nil
}
