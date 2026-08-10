package memory

import (
	"strings"
	"testing"
)

func TestApplicabilitySubjectAndOperator(t *testing.T) {
	for _, s := range []ApplicabilitySubject{ApplicabilitySubjectEnvironment, ApplicabilitySubjectProject, ApplicabilitySubjectToolchain, ApplicabilitySubjectComponent} {
		if err := s.Validate(); err != nil {
			t.Errorf("subject %q should be valid: %v", s, err)
		}
	}
	for _, s := range []ApplicabilitySubject{"", "os", "component_extra"} {
		if err := s.Validate(); err == nil {
			t.Errorf("subject %q should be rejected", s)
		}
	}
	for _, o := range []ApplicabilityOperator{ApplicabilityOperatorEquals, ApplicabilityOperatorNotEquals, ApplicabilityOperatorContains, ApplicabilityOperatorExists, ApplicabilityOperatorVersionSatisfies} {
		if err := o.Validate(); err != nil {
			t.Errorf("operator %q should be valid: %v", o, err)
		}
	}
	for _, o := range []ApplicabilityOperator{"", "=", "matches", "regex"} {
		if err := o.Validate(); err == nil {
			t.Errorf("operator %q should be rejected", o)
		}
	}
}

func TestConditionValueScalarKinds(t *testing.T) {
	vals := []ConditionValue{
		StrConditionValue("modernc.org/sqlite"),
		BoolConditionValue(true),
		NumConditionValue(3.14),
		NumConditionValue(42),
		ArrayConditionValue(StrScalar("a"), NumScalar(1), BoolScalar(false)),
	}
	for _, v := range vals {
		if err := v.Validate(); err != nil {
			t.Errorf("value %+v should be valid: %v", v, err)
		}
	}
}

func TestConditionValueBounds(t *testing.T) {
	tooLong := StrConditionValue(strings.Repeat("x", 257))
	if err := tooLong.Validate(); err == nil {
		t.Error("string value over limit should be rejected")
	}
	var elems []ConditionScalar
	for i := 0; i < 33; i++ {
		elems = append(elems, StrScalar("x"))
	}
	if err := ArrayConditionValue(elems...).Validate(); err == nil {
		t.Error("array value over bound should be rejected")
	}
	if err := ArrayConditionValue().Validate(); err == nil {
		t.Error("empty array value should be rejected")
	}
	empty := ConditionValue{}
	if err := empty.Validate(); err == nil {
		t.Error("empty condition value should be rejected")
	}
	both := ConditionValue{Scalar: &ConditionScalar{Str: strPtr("a")}, Array: []ConditionScalar{StrScalar("b")}}
	if err := both.Validate(); err == nil {
		t.Error("condition value with scalar and array should be rejected")
	}
}

func TestConditionValueNaNInfRejected(t *testing.T) {
	nan := NumConditionValue(0)
	*nan.Scalar.Num = nanValue()
	if err := nan.Validate(); err == nil {
		t.Error("NaN value should be rejected")
	}
	inf := NumConditionValue(1)
	*inf.Scalar.Num = infValue()
	if err := inf.Validate(); err == nil {
		t.Error("infinite value should be rejected")
	}
}

func TestConditionValueNullRejected(t *testing.T) {
	in := `{"condition_id":"c1","subject":"environment","subject_ref":null,"field":"sqlite_driver","operator":"equals","value":null}`
	if _, err := DecodeStrict[ApplicabilityCondition]([]byte(in)); err == nil {
		t.Error("null condition value must be rejected")
	}
	in2 := `{"condition_id":"c1","subject":"environment","subject_ref":null,"field":"sqlite_driver","operator":"equals","value":"ok"}`
	if _, err := DecodeStrict[ApplicabilityCondition]([]byte(in2)); err != nil {
		t.Errorf("valid scalar condition rejected: %v", err)
	}
	// null inside a value array must be rejected too.
	in3 := `{"condition_id":"c1","subject":"environment","subject_ref":null,"field":"sqlite_driver","operator":"equals","value":["ok",null]}`
	if _, err := DecodeStrict[ApplicabilityCondition]([]byte(in3)); err == nil {
		t.Error("null inside condition value array must be rejected")
	}
}

func TestApplicabilityConditionValidation(t *testing.T) {
	compRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeComponent, MemoryID: "mem_comp_01", Revision: 1, ContentSHA256: testHash}
	base := ApplicabilityCondition{
		ConditionID: "condition_sqlite_driver",
		Subject:     ApplicabilitySubjectEnvironment,
		Field:       "sqlite_driver",
		Operator:    ApplicabilityOperatorEquals,
		Value:       StrConditionValue("modernc.org/sqlite"),
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid condition rejected: %v", err)
	}
	comp := base
	comp.Subject = ApplicabilitySubjectComponent
	comp.SubjectRef = &compRef
	if err := comp.Validate(); err != nil {
		t.Errorf("component condition with ref should be valid: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*ApplicabilityCondition)
	}{
		{"empty condition_id", func(c *ApplicabilityCondition) { c.ConditionID = "" }},
		{"path condition_id", func(c *ApplicabilityCondition) { c.ConditionID = "../cond" }},
		{"invalid subject", func(c *ApplicabilityCondition) { c.Subject = ApplicabilitySubject("os") }},
		{"component missing subject_ref", func(c *ApplicabilityCondition) { c.Subject = ApplicabilitySubjectComponent }},
		{"component ref of wrong type", func(c *ApplicabilityCondition) {
			c.Subject = ApplicabilitySubjectComponent
			wrong := compRef
			wrong.MemoryType = MemoryTypeStrategy
			c.SubjectRef = &wrong
		}},
		{"non-component carrying subject_ref", func(c *ApplicabilityCondition) {
			r := compRef
			c.SubjectRef = &r
		}},
		{"invalid field", func(c *ApplicabilityCondition) { c.Field = "go version" }},
		{"field with absolute path", func(c *ApplicabilityCondition) { c.Field = "/etc/passwd" }},
		{"field with newline", func(c *ApplicabilityCondition) { c.Field = "field\nrun_me" }},
		{"field with command content", func(c *ApplicabilityCondition) { c.Field = "x; rm -rf /" }},
		{"field with shell expansion", func(c *ApplicabilityCondition) { c.Field = "$(curl evil)" }},
		{"invalid operator", func(c *ApplicabilityCondition) { c.Operator = ApplicabilityOperator("regex") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cond := base
			c.mut(&cond)
			if err := cond.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

// strPtr returns a pointer to s. Defined here to keep helpers next to use.
func strPtr(s string) *string { return &s }
