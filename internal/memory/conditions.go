package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ApplicabilitySubject is the schema-bound subject of an applicability
// condition.
type ApplicabilitySubject string

const (
	ApplicabilitySubjectEnvironment ApplicabilitySubject = "environment"
	ApplicabilitySubjectProject     ApplicabilitySubject = "project"
	ApplicabilitySubjectToolchain   ApplicabilitySubject = "toolchain"
	ApplicabilitySubjectComponent   ApplicabilitySubject = "component"
)

func (s ApplicabilitySubject) Validate() error {
	switch s {
	case ApplicabilitySubjectEnvironment, ApplicabilitySubjectProject,
		ApplicabilitySubjectToolchain, ApplicabilitySubjectComponent:
		return nil
	default:
		return fmt.Errorf("invalid applicability subject %q", s)
	}
}

// ApplicabilityOperator is the schema-bound operator of an applicability
// condition.
type ApplicabilityOperator string

const (
	ApplicabilityOperatorEquals           ApplicabilityOperator = "equals"
	ApplicabilityOperatorNotEquals        ApplicabilityOperator = "not_equals"
	ApplicabilityOperatorContains         ApplicabilityOperator = "contains"
	ApplicabilityOperatorExists           ApplicabilityOperator = "exists"
	ApplicabilityOperatorVersionSatisfies ApplicabilityOperator = "version_satisfies"
)

func (o ApplicabilityOperator) Validate() error {
	switch o {
	case ApplicabilityOperatorEquals, ApplicabilityOperatorNotEquals,
		ApplicabilityOperatorContains, ApplicabilityOperatorExists,
		ApplicabilityOperatorVersionSatisfies:
		return nil
	default:
		return fmt.Errorf("invalid applicability operator %q", o)
	}
}

// ConditionScalar is a single allowed condition value: string, bool or
// finite number.
type ConditionScalar struct {
	Str  *string
	Bool *bool
	Num  *float64
}

func (s ConditionScalar) MarshalJSON() ([]byte, error) {
	switch {
	case s.Str != nil:
		return json.Marshal(*s.Str)
	case s.Bool != nil:
		return json.Marshal(*s.Bool)
	case s.Num != nil:
		return json.Marshal(*s.Num)
	default:
		return nil, errors.New("memory: empty condition scalar")
	}
}

func (s *ConditionScalar) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if string(trimmed) == "null" {
		return errors.New("memory: condition scalar must not be null")
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Str = &str
		return nil
	}
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		s.Bool = &b
		return nil
	}
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		s.Num = &n
		return nil
	}
	return errors.New("memory: condition scalar must be a string, bool or number")
}

func (s ConditionScalar) validate() error {
	switch {
	case s.Str != nil:
		return validateText(*s.Str, maxConditionStrLen, "condition string value", false)
	case s.Bool != nil:
		return nil
	case s.Num != nil:
		if math.IsNaN(*s.Num) || math.IsInf(*s.Num, 0) {
			return errors.New("condition number value must be finite")
		}
		return nil
	default:
		return errors.New("condition scalar must have exactly one value")
	}
}

func (s ConditionScalar) canonValue() any {
	switch {
	case s.Str != nil:
		return *s.Str
	case s.Bool != nil:
		return *s.Bool
	default:
		return *s.Num
	}
}

// ConditionValue is a scalar or a bounded array of scalars.
type ConditionValue struct {
	Scalar *ConditionScalar  `json:"-"`
	Array  []ConditionScalar `json:"-"`
}

func StrConditionValue(s string) ConditionValue {
	return ConditionValue{Scalar: &ConditionScalar{Str: &s}}
}

func BoolConditionValue(b bool) ConditionValue {
	return ConditionValue{Scalar: &ConditionScalar{Bool: &b}}
}

func NumConditionValue(n float64) ConditionValue {
	return ConditionValue{Scalar: &ConditionScalar{Num: &n}}
}

func ArrayConditionValue(vals ...ConditionScalar) ConditionValue {
	return ConditionValue{Array: vals}
}

func StrScalar(s string) ConditionScalar  { return ConditionScalar{Str: &s} }
func BoolScalar(b bool) ConditionScalar   { return ConditionScalar{Bool: &b} }
func NumScalar(n float64) ConditionScalar { return ConditionScalar{Num: &n} }

func (v ConditionValue) MarshalJSON() ([]byte, error) {
	switch {
	case v.Scalar != nil:
		return json.Marshal(v.Scalar)
	case v.Array != nil:
		return json.Marshal(v.Array)
	default:
		return nil, errors.New("memory: empty condition value")
	}
}

func (v *ConditionValue) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("memory: empty condition value")
	}
	if trimmed[0] == '[' {
		var arr []ConditionScalar
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		v.Array = arr
		return nil
	}
	var s ConditionScalar
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v.Scalar = &s
	return nil
}

func (v ConditionValue) Validate() error {
	switch {
	case v.Scalar != nil:
		if v.Array != nil {
			return errors.New("condition value must be either scalar or array, not both")
		}
		return v.Scalar.validate()
	case v.Array != nil:
		if len(v.Array) == 0 {
			return errors.New("condition value array must not be empty")
		}
		if len(v.Array) > maxConditionArrLen {
			return fmt.Errorf("condition value array exceeds %d elements", maxConditionArrLen)
		}
		for _, s := range v.Array {
			if err := s.validate(); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("condition value must be a scalar or array")
	}
}

func (v ConditionValue) canonValue() (any, error) {
	switch {
	case v.Scalar != nil:
		return v.Scalar.canonValue(), nil
	case v.Array != nil:
		out := make([]any, 0, len(v.Array))
		for _, s := range v.Array {
			out = append(out, s.canonValue())
		}
		return out, nil
	default:
		return nil, errors.New("memory: empty condition value")
	}
}

// ApplicabilityCondition is a strictly structured, machine-readable
// condition. Free-text machine conditions are not allowed.
type ApplicabilityCondition struct {
	ConditionID string                `json:"condition_id"`
	Subject     ApplicabilitySubject  `json:"subject"`
	SubjectRef  *MemoryRef            `json:"subject_ref"`
	Field       string                `json:"field"`
	Operator    ApplicabilityOperator `json:"operator"`
	Value       ConditionValue        `json:"value"`
}

func (c ApplicabilityCondition) Validate() error {
	if err := validateID(c.ConditionID, "condition_id"); err != nil {
		return fmt.Errorf("applicability condition: %w", err)
	}
	if err := c.Subject.Validate(); err != nil {
		return fmt.Errorf("applicability condition: %w", err)
	}
	if c.Subject == ApplicabilitySubjectComponent {
		if c.SubjectRef == nil {
			return errors.New("applicability condition: component subject requires subject_ref")
		}
		if err := c.SubjectRef.Validate(); err != nil {
			return fmt.Errorf("applicability condition: %w", err)
		}
		if c.SubjectRef.MemoryType != MemoryTypeComponent {
			return errors.New("applicability condition: component subject_ref must reference memory_type component")
		}
	} else if c.SubjectRef != nil {
		return errors.New("applicability condition: subject_ref must be null unless subject is component")
	}
	if err := validateField(c.Field); err != nil {
		return fmt.Errorf("applicability condition: %w", err)
	}
	if err := c.Operator.Validate(); err != nil {
		return fmt.Errorf("applicability condition: %w", err)
	}
	return c.Value.Validate()
}

func (c ApplicabilityCondition) canonMap() (map[string]any, error) {
	var subjectRef any
	if c.SubjectRef != nil {
		ref, err := c.SubjectRef.canonMap()
		if err != nil {
			return nil, err
		}
		subjectRef = ref
	}
	value, err := c.Value.canonValue()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"condition_id": c.ConditionID,
		"subject":      string(c.Subject),
		"subject_ref":  subjectRef,
		"field":        c.Field,
		"operator":     string(c.Operator),
		"value":        value,
	}, nil
}

func (c ApplicabilityCondition) CanonicalBytes() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (c ApplicabilityCondition) ContentHash() (string, error) {
	b, err := c.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (c ApplicabilityCondition) EncodeCanonical() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}
