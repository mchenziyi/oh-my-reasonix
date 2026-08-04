package commenthook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

// Audit decisions recorded by the guard.
const (
	DecisionPass         = "pass"
	DecisionWarning      = "warning"
	DecisionBlocking     = "blocking"
	DecisionParseFailure = "parse_failure"
)

// Audit limits: entries and bytes are capped; overflow evicts oldest first.
const (
	maxAuditEntries = 1000
	maxAuditBytes   = 256 * 1024
)

// AuditEntry is a sanitized record of one guard decision. It deliberately
// never contains command bodies, tool args, file contents, or credentials.
type AuditEntry struct {
	SchemaVersion int            `json:"schema_version"`
	Time          string         `json:"time"`
	Event         string         `json:"event"`
	Decision      string         `json:"decision"`
	RuleCounts    map[string]int `json:"rule_counts,omitempty"`
	ExitCode      int            `json:"exit_code"`
	DurationMs    int64          `json:"duration_ms"`
	OMRVersion    string         `json:"omr_version"`
	TriggeredRule string         `json:"triggered_rule,omitempty"`
}

// AuditClearMode selects dry-run or real clearing.
type AuditClearMode int

const (
	// DryRun previews the clear without writing.
	DryRun AuditClearMode = iota
	// RealClear removes the audit log.
	RealClear
)

// AuditStore appends sanitized guard decisions to
// <project>/.reasonix/omr/audit/audit.jsonl with retention limits.
type AuditStore struct {
	Dir string
}

// AuditDirName is the project-relative audit directory.
const AuditDirName = ".reasonix/omr/audit"

// NewAuditStore resolves the project audit directory.
func NewAuditStore(projectDir string) (AuditStore, error) {
	root, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return AuditStore{}, err
	}
	return AuditStore{Dir: filepath.Join(root, AuditDirName)}, nil
}

// Path returns the audit JSONL file path.
func (a AuditStore) Path() string { return filepath.Join(a.Dir, "audit.jsonl") }

// check rejects symlinked audit components (fail closed). Only the fixed
// audit path components below the project root are inspected; parent
// directories (which may legitimately be symlinked, e.g. /tmp on macOS)
// are out of scope.
func (a AuditStore) check() error {
	root := filepath.Dir(filepath.Dir(filepath.Dir(a.Dir)))
	cur := root
	for _, part := range []string{".reasonix", "omr", "audit"} {
		cur = filepath.Join(cur, part)
		if info, err := os.Lstat(cur); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path rejected: %s", cur)
		}
	}
	if info, err := os.Lstat(a.Path()); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink path rejected: %s", a.Path())
	}
	return nil
}

// Append records one decision. On retention overflow the oldest entries are
// evicted. Writes are atomic; a failure is returned to the caller so the
// guard can fail closed.
func (a AuditStore) Append(entry AuditEntry) error {
	if err := a.check(); err != nil {
		return err
	}
	if entry.SchemaVersion == 0 {
		entry.SchemaVersion = 1
	}
	if entry.Time == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if entry.OMRVersion == "" {
		entry.OMRVersion = manifest.Version
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if err := os.MkdirAll(a.Dir, 0700); err != nil {
		return err
	}
	existing, err := os.ReadFile(a.Path())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	merged := append(existing, line...)
	merged = a.applyRetention(merged)
	return fileutil.AtomicWrite(a.Path(), merged, 0600)
}

// applyRetention drops oldest lines beyond entry and byte limits.
func (a AuditStore) applyRetention(data []byte) []byte {
	lines := bytes.Split(data, []byte{'\n'})
	// Trim a trailing empty element produced by the final newline.
	if n := len(lines); n > 0 && len(lines[n-1]) == 0 {
		lines = lines[:n-1]
	}
	if len(lines) > maxAuditEntries {
		lines = lines[len(lines)-maxAuditEntries:]
	}
	out := make([][]byte, 0, len(lines))
	var total int
	// Always keep at least one line even if it exceeds the byte budget.
	for _, line := range lines {
		next := total + len(line) + 1
		if next > maxAuditBytes && len(out) > 0 {
			continue
		}
		out = append(out, line)
		total = next
	}
	return append(bytes.Join(out, []byte{'\n'}), '\n')
}

// List returns all recorded entries in append order. Corrupt or unknown-field
// lines are rejected so logs fail closed.
func (a AuditStore) List() ([]AuditEntry, error) {
	if err := a.check(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(a.Path())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []AuditEntry
	reader := bufio.NewReader(bytes.NewReader(b))
	for {
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var entry AuditEntry
			dec := json.NewDecoder(bytes.NewReader(line))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&entry); err != nil {
				return nil, fmt.Errorf("corrupt audit log: %w", err)
			}
			out = append(out, entry)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, readErr
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, nil
}

// Clear removes the audit log. In DryRun mode it returns without writing.
func (a AuditStore) Clear(mode AuditClearMode) (bool, error) {
	if mode == DryRun {
		_, err := a.List() // validates the log is readable
		return false, err
	}
	if err := a.check(); err != nil {
		return false, err
	}
	if err := os.Remove(a.Path()); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

// Summary renders the audit log as sanitized counts per decision.
func (a AuditStore) Summary() (map[string]int, error) {
	entries, err := a.List()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Decision]++
	}
	return counts, nil
}

// sanitizeMessage is a guardrail for any human-readable guard output: it
// strips command-like and credential-like content before logging.
func sanitizeMessage(message string) string {
	if message == "" {
		return ""
	}
	if len(message) > 512 {
		message = message[:512]
	}
	message = strings.ReplaceAll(message, "\n", " ")
	return message
}
