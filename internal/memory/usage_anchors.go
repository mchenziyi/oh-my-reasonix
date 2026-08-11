package memory

// MEM-02A: MemoryUsage evaluation anchor value objects. These are strict,
// reusable value types pinned by the Schema Convergence decisions (D10/D11):
// MemoryContext reuses the frozen Project/GlobalGenerationRef pair (no
// generic GenerationRef, no CURRENT reads); ObservationProvenance enforces
// the architecture 12.2 source/ref matrix. Neither object holds free text,
// prompts, commands, thoughts or credentials.

import (
	"errors"
	"fmt"
)

// MemoryContext pins the Project/Global Generation Pair of one evaluation
// world. At least one side must be present; the other side is explicitly
// null when that scope had no memory at the time. Structure-only validation:
// this stage never reads CURRENT nor checks "latest".
type MemoryContext struct {
	ProjectGenerationRef *ProjectGenerationRef `json:"project_generation_ref"`
	GlobalGenerationRef  *GlobalGenerationRef  `json:"global_generation_ref"`
}

func (c MemoryContext) Validate() error {
	if c.ProjectGenerationRef == nil && c.GlobalGenerationRef == nil {
		return errors.New("memory context: at least one of project/global generation ref is required")
	}
	if c.ProjectGenerationRef != nil {
		if err := c.ProjectGenerationRef.Validate(); err != nil {
			return fmt.Errorf("memory context: %w", err)
		}
	}
	if c.GlobalGenerationRef != nil {
		if err := c.GlobalGenerationRef.Validate(); err != nil {
			return fmt.Errorf("memory context: %w", err)
		}
	}
	return nil
}

// canonMap renders exactly two keys; a missing side is an explicit null so a
// nil side can never be confused with an empty object.
func (c MemoryContext) canonMap() (map[string]any, error) {
	var prj, glb any
	if c.ProjectGenerationRef != nil {
		prj = c.ProjectGenerationRef.canonMap()
	}
	if c.GlobalGenerationRef != nil {
		glb = c.GlobalGenerationRef.canonMap()
	}
	return map[string]any{
		"project_generation_ref": prj,
		"global_generation_ref":  glb,
	}, nil
}

// ObservationProvenance records how a usage observation was obtained and
// where the evidence lives, per architecture 12.2:
//
//	source             evidence_ref          judgment_ref
//	agent_reported     required              must be null
//	runtime_observed   required              must be null
//	user_confirmed     optional              required, confirmation
//
// No free text is stored anywhere in this object.
type ObservationProvenance struct {
	Source      string       `json:"source"`
	EvidenceRef *EvidenceRef `json:"evidence_ref"`
	JudgmentRef *JudgmentRef `json:"judgment_ref"`
}

func (p ObservationProvenance) Validate() error {
	switch p.Source {
	case "agent_reported", "runtime_observed":
		if p.EvidenceRef == nil {
			return fmt.Errorf("observation provenance: %s requires an evidence_ref", p.Source)
		}
		if p.JudgmentRef != nil {
			return errors.New("observation provenance: agent_reported/runtime_observed must not carry a judgment_ref")
		}
		if err := p.EvidenceRef.Validate(); err != nil {
			return fmt.Errorf("observation provenance: %w", err)
		}
	case "user_confirmed":
		if p.JudgmentRef == nil {
			return errors.New("observation provenance: user_confirmed requires a judgment_ref")
		}
		if p.JudgmentRef.JudgmentType != JudgmentTypeConfirmation {
			return errors.New("observation provenance: user_confirmed judgment_ref must be a confirmation judgment")
		}
		if err := p.JudgmentRef.Validate(); err != nil {
			return fmt.Errorf("observation provenance: %w", err)
		}
		if p.EvidenceRef != nil {
			if err := p.EvidenceRef.Validate(); err != nil {
				return fmt.Errorf("observation provenance: %w", err)
			}
		}
	default:
		return errors.New("observation provenance: unknown source")
	}
	return nil
}

func (p ObservationProvenance) canonMap() (map[string]any, error) {
	var ev, jd any
	if p.EvidenceRef != nil {
		m, err := p.EvidenceRef.canonMap()
		if err != nil {
			return nil, err
		}
		ev = m
	}
	if p.JudgmentRef != nil {
		m, err := p.JudgmentRef.canonMap()
		if err != nil {
			return nil, err
		}
		jd = m
	}
	return map[string]any{
		"source":       p.Source,
		"evidence_ref": ev,
		"judgment_ref": jd,
	}, nil
}
