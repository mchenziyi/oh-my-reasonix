package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Schema version for all Mnemosyne fact documents (Architecture v1).
const SchemaVersion = 1

// HashPrefix is the mandatory prefix of every program-computed content hash.
const HashPrefix = "sha256_"

const hashHexLen = 64

// Content-size limits. They are deliberate anti-abuse bounds, not semantics.
const (
	maxTitleLen        = 200
	maxSummaryLen      = 20000
	maxAliasLen        = 200
	maxAliases         = 50
	maxConditions      = 64
	maxConditionStrLen = 256
	maxConditionArrLen = 32
	maxRefs            = 32
	maxRelations       = 64
	maxBasisRefs       = 64
	maxReasonLen       = 2000
	maxPayloadRefs     = 64
	maxIDLen           = 128
)

var (
	reIdentifier   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_@:.-]*$`)
	reCanonicalKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	reField        = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
	reVersionID    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)
	reHash         = regexp.MustCompile(`^sha256_[0-9a-f]{64}$`)
)

// Fact is implemented by every immutable Mnemosyne fact document.
type Fact interface {
	Validate() error
	CanonicalBytes() ([]byte, error)
	ContentHash() (string, error)
	EncodeCanonical() ([]byte, error)
}

// canonMappable is implemented by every type that can render itself as a
// deterministic map for canonical JSON encoding. Maps are marshaled by
// encoding/json in sorted key order, so no map iteration order can leak into
// the hash.
type canonMappable interface {
	canonMap() (map[string]any, error)
}

// DecodeStrict decodes a single fact document with unknown-field rejection,
// trailing-data rejection and full validation (including hash verification).
func DecodeStrict[T Fact](data []byte) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("memory: decode: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return v, errors.New("memory: decode: trailing data after JSON document")
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	return v, nil
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return HashPrefix + hex.EncodeToString(sum[:])
}

// NewContentHash computes the canonical content hash of arbitrary canonical
// bytes. Used by tests and tooling; facts compute their own hashes.
func NewContentHash(data []byte) string {
	return hashOf(data)
}

// validateHash enforces the sha256_ + 64 lowercase hex format.
func validateHash(s, what string) error {
	if !reHash.MatchString(s) {
		return fmt.Errorf("%s %q is not a valid %s hash", what, s, HashPrefix)
	}
	return nil
}

// validateID rejects empty ids, path-like ids, whitespace, control characters
// and anything that could be confused with a file path.
func validateID(s, what string) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", what)
	}
	if len(s) > maxIDLen {
		return fmt.Errorf("%s exceeds %d bytes", what, maxIDLen)
	}
	if strings.Contains(s, "..") || !reIdentifier.MatchString(s) {
		return fmt.Errorf("%s %q is not a valid identifier", what, s)
	}
	return nil
}

func validateCanonicalKey(s string) error {
	if s == "" {
		return errors.New("canonical_key must not be empty")
	}
	if len(s) > maxIDLen {
		return fmt.Errorf("canonical_key exceeds %d bytes", maxIDLen)
	}
	if !reCanonicalKey.MatchString(s) {
		return fmt.Errorf("canonical_key %q is not a valid stable key", s)
	}
	return nil
}

// validateField enforces schema-bound identifiers: no absolute paths, no
// newlines, no command content, no free instructions.
func validateField(s string) error {
	if s == "" {
		return errors.New("field must not be empty")
	}
	if len(s) > maxIDLen {
		return fmt.Errorf("field exceeds %d bytes", maxIDLen)
	}
	if !reField.MatchString(s) {
		return fmt.Errorf("field %q is not a schema-bound identifier", s)
	}
	return nil
}

// validateVersionID allows slashes so version strings like
// "mnemosyne-compiler/1" remain expressible, but rejects paths and traversal.
func validateVersionID(s, what string) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", what)
	}
	if len(s) > maxIDLen {
		return fmt.Errorf("%s exceeds %d bytes", what, maxIDLen)
	}
	if strings.Contains(s, "..") || !reVersionID.MatchString(s) {
		return fmt.Errorf("%s %q is not a valid version identifier", what, s)
	}
	return nil
}

func validateText(s string, maxLen int, what string, allowNewline bool) error {
	if len(s) > maxLen {
		return fmt.Errorf("%s exceeds %d bytes", what, maxLen)
	}
	for _, r := range s {
		if r < 0x20 && (r != '\n' || !allowNewline) {
			return fmt.Errorf("%s contains control characters", what)
		}
		if r == 0x7f {
			return fmt.Errorf("%s contains control characters", what)
		}
	}
	return nil
}

func validateTitle(s string) error {
	if s == "" {
		return errors.New("title must not be empty")
	}
	return validateText(s, maxTitleLen, "title", false)
}

func validateSummary(s string) error {
	if s == "" {
		return errors.New("summary must not be empty")
	}
	return validateText(s, maxSummaryLen, "summary", true)
}

func validateAlias(s string) error {
	if s == "" {
		return errors.New("alias must not be empty")
	}
	return validateText(s, maxAliasLen, "alias", false)
}

func validateTime(s, what string) error {
	if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
		return fmt.Errorf("%s %q is not RFC3339Nano", what, s)
	}
	return nil
}

// normalizeTime renders an RFC3339Nano timestamp in canonical UTC form so
// equivalent instants with different offsets hash identically.
func normalizeTime(s string) (string, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "", fmt.Errorf("time %q is not RFC3339Nano: %w", s, err)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

// sortDedupe sorts items by their canonical JSON key and removes adjacent
// duplicates. The comparison is a total order on the keys, so the result is
// deterministic regardless of input order.
func sortDedupe[T any](items []T, key func(T) (string, error)) ([]T, error) {
	if len(items) < 2 {
		return items, nil
	}
	type entry struct {
		key  string
		item T
	}
	entries := make([]entry, 0, len(items))
	for _, it := range items {
		k, err := key(it)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{key: k, item: it})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	out := make([]T, 0, len(entries))
	for i, e := range entries {
		if i > 0 && entries[i-1].key == e.key {
			continue
		}
		out = append(out, e.item)
	}
	return out, nil
}

// canonSlice sorts and deduplicates a set-typed slice and renders it as a
// deterministic JSON array. nil renders as [].
func canonSlice[T canonMappable](items []T) ([]any, error) {
	if items == nil {
		return []any{}, nil
	}
	sorted, err := sortDedupe(items, func(it T) (string, error) {
		m, err := it.canonMap()
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(m)
		return string(b), err
	})
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(sorted))
	for _, it := range sorted {
		m, err := it.canonMap()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func canonStrings(items []string) ([]any, error) {
	if items == nil {
		return []any{}, nil
	}
	sorted, err := sortDedupe(items, func(s string) (string, error) { return s, nil })
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, s)
	}
	return out, nil
}
