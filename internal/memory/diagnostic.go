package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Code is a stable, machine-readable diagnostic code. Codes are part of the
// Store contract; callers must never rely on error text.
type Code string

const (
	CodeNotFound            Code = "memory_store_not_found"
	CodePathUnsafe          Code = "memory_store_path_unsafe"
	CodeSymlinkRejected     Code = "memory_store_symlink_rejected"
	CodeScopeMismatch       Code = "memory_store_scope_mismatch"
	CodePermissionDenied    Code = "memory_store_permission_denied"
	CodeInvalidJSON         Code = "memory_store_invalid_json"
	CodeUnknownField        Code = "memory_store_unknown_field"
	CodeSchemaInvalid       Code = "memory_store_schema_invalid"
	CodeHashMismatch        Code = "memory_store_hash_mismatch"
	CodeIdentityConflict    Code = "memory_store_identity_conflict"
	CodeInsecurePermissions Code = "memory_store_insecure_permissions"
	// CodeImmutableConflict is reserved for later phases (MEM-01D prepared
	// transactions and freeze semantics); it has no producer in MEM-01B.
	CodeImmutableConflict Code = "memory_store_immutable_conflict"
	CodeLockTimeout       Code = "memory_store_lock_timeout"
	CodeCorruptFile       Code = "memory_store_corrupt_file"
	// Generation transaction codes (MEM-01D).
	CodeGenerationTxConflict          Code = "memory_generation_transaction_conflict"
	CodeGenerationIdempotency         Code = "memory_generation_idempotency_conflict"
	CodeGenerationCurrentCAS          Code = "memory_generation_current_cas_conflict"
	CodeGenerationManifestMismatch    Code = "memory_generation_manifest_mismatch"
	CodeGenerationStagingInvalid      Code = "memory_generation_staging_invalid"
	CodeGenerationCompilerUnavailable Code = "memory_generation_compiler_unavailable"
	CodeGenerationRecoveryBlocked     Code = "memory_generation_recovery_blocked"
	CodeGenerationAlreadyCommitted    Code = "memory_generation_already_committed"
	CodeGenerationRecoveryPending     Code = "memory_generation_recovery_pending"
	CodeGenerationAbortFailed         Code = "memory_generation_abort_failed"
	// OKF compiler codes (MEM-01E).
	CodeOKFInvalidInput Code = "memory_okf_invalid_input"
	CodeOKFCompileError Code = "memory_okf_compile_error"

	CodeDerivedInvalidInput Code = "memory_derived_invalid_input"

	// Evaluation codes (MEM-02-01).
	CodeEvaluationFutureReference Code = "memory_evaluation_future_reference"
)

// Resource limits. These are implementation safety bounds, centralized here
// so they can be reviewed and tested; they never change Schema semantics.
const (
	// DefaultMaxFactBytes limits a single fact JSON file. Oversized files
	// are rejected instead of truncated.
	DefaultMaxFactBytes int64 = 1 << 20 // 1 MiB
	// MaxFactKeyBytes bounds the identity key used by Get/Put (up to three
	// 128-byte ID components plus separators).
	MaxFactKeyBytes = 3*(maxIDLen+1) + maxIDLen
	// DefaultLockTimeout bounds the wait for the store write lock.
	DefaultLockTimeout = 5 * time.Second
	// tempPrefix marks in-flight atomic write files; residuals are reported
	// by Diagnose, never silently deleted.
	tempPrefix = ".omr-fact-"
)

// StoreError is a redacted, code-carrying store error. Msg must never
// contain absolute paths, prompts, credentials or other sensitive content.
type StoreError struct {
	Code Code
	Msg  string
}

func (e *StoreError) Error() string {
	if e.Msg == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func storeError(code Code, msg string) error {
	return &StoreError{Code: code, Msg: msg}
}

// ErrorCode extracts the stable diagnostic code from err, or "" when err is
// not a StoreError.
func ErrorCode(err error) Code {
	var se *StoreError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

// classifyDecodeError maps DecodeStrict and validation errors onto stable
// Store diagnostic codes without leaking message details. The hash-mismatch
// detection matches the exact suffixes emitted by the model validators, so
// unrelated schema messages cannot be misclassified.
func classifyDecodeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unknown field"):
		return storeError(CodeUnknownField, "fact contains unknown fields")
	case isHashMismatchMsg(msg):
		return storeError(CodeHashMismatch, "fact content hash mismatch")
	case strings.Contains(msg, "decode:"):
		return storeError(CodeInvalidJSON, "fact is not valid strict JSON")
	default:
		return storeError(CodeSchemaInvalid, "fact violates the schema")
	}
}

func isHashMismatchMsg(msg string) bool {
	// The model validators always end hash errors with the field name plus
	// " mismatch" and nothing after, so suffix matching closes the
	// injection window where a crafted field value embeds the same text
	// (e.g. an invalid memory_id ending in " is not a valid identifier").
	for _, suffix := range []string{
		"content_sha256 mismatch",
		"evidence_set_sha256 mismatch",
		"input_manifest_sha256 mismatch",
	} {
		if strings.HasSuffix(msg, suffix) {
			return true
		}
	}
	return false
}
