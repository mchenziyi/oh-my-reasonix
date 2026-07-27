// Package grillme provides a pure-Go offline replay model for the omr-grill-me
// Profile. It simulates the session logic declared in SKILL.md (stop conditions,
// assumption handling, output structure) without calling any AI model or
// accessing Reasonix state. Tests use predefined inputs and expected outputs.
package grillme

// StopReason indicates why a Grill Me session ended.
type StopReason string

const (
	StopInfoSufficient StopReason = "info_sufficient"
	StopMaxRounds      StopReason = "max_rounds"
	StopUserRequest    StopReason = "user_request"
)

// MaxRounds is the maximum number of question rounds before forced stop,
// as declared in the SKILL.md stop conditions.
const MaxRounds = 6

// assumptionRecord tracks one assumption, whether the user confirmed it,
// and whether they explicitly rejected it (as opposed to not yet addressed).
type assumptionRecord struct {
	statement string
	confirmed bool
	rejected  bool
}

// Result is the structured output of a Grill Me session, matching the
// YAML output format defined in SKILL.md.
type Result struct {
	AssumptionsConfirmed []string
	Ambiguities          []string
	Gaps                 []string
	Risks                []string
	Recommendation       string
}

// sessionConfig holds the input for one replay scenario.
type sessionConfig struct {
	// roundsCompleted is the number of question-answer exchanges simulated.
	roundsCompleted int
	// userStopped, when true, simulates the user requesting an early stop.
	userStopped bool
	// infoSufficient, when true, simulates that answers are clear enough
	// that no more questions are needed.
	infoSufficient bool
	// assumptions is the list of assumptions with their confirmation status.
	assumptions []assumptionRecord
	// gaps and risks are predefined observations to include in the result.
	gaps  []string
	risks []string
}

// Replay runs one Grill Me session scenario with the given configuration
// and returns the structured Result along with the StopReason.
// This is a pure-Go offline replay — no AI model is invoked.
func Replay(cfg sessionConfig) (Result, StopReason) {
	var result Result
	var stopReason StopReason

	// --- Determine stop reason ---
	switch {
	case cfg.userStopped:
		stopReason = StopUserRequest
	case cfg.infoSufficient:
		stopReason = StopInfoSufficient
	case cfg.roundsCompleted >= MaxRounds:
		stopReason = StopMaxRounds
	default:
		// If none triggered but rounds exist, assume info sufficient.
		stopReason = StopInfoSufficient
	}

	// --- Process assumptions ---
	// confirmed → AssumptionsConfirmed
	// explicitly rejected → neither confirmed nor ambiguous (deliberately discarded)
	// unconfirmed (not yet addressed) → Ambiguities
	hasRejected := false
	for _, a := range cfg.assumptions {
		switch {
		case a.confirmed:
			result.AssumptionsConfirmed = append(result.AssumptionsConfirmed, a.statement)
		case a.rejected:
			hasRejected = true
		default:
			result.Ambiguities = append(result.Ambiguities, "unconfirmed: "+a.statement)
		}
	}

	// --- Copy gaps and risks ---
	result.Gaps = append([]string(nil), cfg.gaps...)
	result.Risks = append([]string(nil), cfg.risks...)

	// --- Determine recommendation ---
	switch {
	case stopReason == StopUserRequest:
		result.Recommendation = "pause"
	case len(result.Ambiguities) > 0 || len(result.Gaps) > 0:
		result.Recommendation = "pause"
	case len(result.AssumptionsConfirmed) == 0 && hasRejected && len(result.Ambiguities) == 0:
		// All assumptions were explicitly rejected, none confirmed, no open ambiguities → rethink.
		result.Recommendation = "rethink"
	default:
		result.Recommendation = "proceed"
	}

	return result, stopReason
}
