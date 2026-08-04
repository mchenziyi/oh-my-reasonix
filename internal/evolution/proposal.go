package evolution

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParseProposal accepts only the documented machine-readable proposal object.
// It deliberately rejects extra fields and never executes or writes its input.
func ParseProposal(data []byte) (Proposal, error) {
	var p Proposal
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Proposal{}, fmt.Errorf("invalid proposal JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Proposal{}, err
	}
	return p, nil
}
