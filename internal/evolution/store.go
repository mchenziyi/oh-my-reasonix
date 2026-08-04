package evolution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

type Store struct {
	Root    string
	ScopeID string
}

func NewStore(projectDir string) (Store, error) {
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return Store{}, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		root = resolved
	}
	return Store{Root: filepath.Join(root, ".reasonix", "omr", "evolution"), ScopeID: NewID("scope", root)}, nil
}

type scopeRecord struct {
	SchemaVersion int    `json:"schema_version"`
	ScopeID       string `json:"scope_id"`
}

func (s Store) checkScope() error {
	p, err := s.safeJoin("scope.json")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var record scopeRecord
	if err := json.Unmarshal(b, &record); err != nil {
		return fmt.Errorf("invalid evolution scope: %w", err)
	}
	if record.SchemaVersion != SchemaVersion || record.ScopeID != s.ScopeID {
		return fmt.Errorf("evolution store belongs to another project scope")
	}
	return nil
}

func (s Store) ensureScope() error {
	if err := s.checkScope(); err != nil {
		return err
	}
	p, err := s.safeJoin("scope.json")
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := Encode(scopeRecord{SchemaVersion: SchemaVersion, ScopeID: s.ScopeID})
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWrite(p, b, 0600)
}

// safeJoin rejects symlink components and traversal below the store root.
func (s Store) safeJoin(parts ...string) (string, error) {
	p := filepath.Join(append([]string{s.Root}, parts...)...)
	rel, err := filepath.Rel(s.Root, p)
	if err != nil || rel == ".." || len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("path escapes evolution root")
	}
	base := filepath.Dir(filepath.Dir(filepath.Dir(s.Root)))
	cur := base
	for _, part := range strings.Split(filepath.Clean(p)[len(base):], string(filepath.Separator)) {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		if info, e := os.Lstat(cur); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path rejected: %s", cur)
		}
	}
	cur2 := s.Root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur2 = filepath.Join(cur2, part)
		if info, e := os.Lstat(cur2); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path rejected: %s", cur2)
		}
	}
	return p, nil
}

func (s Store) write(name string, value any) error {
	if name != "scope.json" {
		if err := s.ensureScope(); err != nil {
			return err
		}
	}
	p, err := s.safeJoin(name)
	if err != nil {
		return err
	}
	b, err := Encode(value)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWrite(p, b, 0600)
}
func (s Store) writeBytes(name string, b []byte) error {
	if name != "scope.json" {
		if err := s.ensureScope(); err != nil {
			return err
		}
	}
	p, err := s.safeJoin(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWrite(p, b, 0600)
}

func (s Store) read(name string, out any) error {
	if name != "scope.json" {
		if err := s.checkScope(); err != nil {
			return err
		}
	}
	p, err := s.safeJoin(name)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func (s Store) RecordEpisode(e Episode) error {
	if err := e.Validate(); err != nil {
		return err
	}
	return s.write(filepath.Join("episodes", e.ID+".json"), e)
}
func (s Store) SavePattern(p Pattern) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return s.write(filepath.Join("patterns", p.ID+".json"), p)
}
func (s Store) SaveProposal(p Proposal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return s.write(filepath.Join("proposals", p.ID+".json"), p)
}
func (s Store) SaveExperiment(e Experiment) error {
	if err := e.Validate(); err != nil {
		return err
	}
	return s.write(filepath.Join("experiments", e.ID+".json"), e)
}

func (s Store) SaveObservation(o Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	return s.write(filepath.Join("observations", o.ID+".json"), o)
}

func (s Store) WriteOverlay(content string) error {
	if err := s.checkScope(); err != nil {
		return err
	}
	if len(content) > 65536 {
		return fmt.Errorf("invalid overlay")
	}
	if content == "" {
		p, err := s.safeJoin("overlay.md")
		if err != nil {
			return err
		}
		if e := os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
		return nil
	}
	return s.writeBytes("overlay.md", []byte(content))
}
func (s Store) ReadOverlay() (string, error) {
	p, err := s.safeJoin("overlay.md")
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	return string(b), err
}
func (s Store) SnapshotOverlay(id string) error {
	content, err := s.ReadOverlay()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		content = ""
	}
	return s.writeBytes(filepath.Join("snapshots", id+".md"), []byte(content))
}
func (s Store) RestoreOverlay(id string) error {
	if err := s.checkScope(); err != nil {
		return err
	}
	p, err := s.safeJoin(filepath.Join("snapshots", id+".md"))
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return s.WriteOverlay(string(b))
}
func (s Store) LoadProposal(id string) (Proposal, error) {
	var p Proposal
	err := s.read(filepath.Join("proposals", id+".json"), &p)
	return p, err
}

func (s Store) list(dir string, out func([]byte) error) error {
	if err := s.checkScope(); err != nil {
		return err
	}
	p, err := s.safeJoin(dir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(p, e.Name()))
		if err != nil {
			return err
		}
		if err := out(b); err != nil {
			return err
		}
	}
	return nil
}
func (s Store) ListEpisodes() ([]Episode, error) {
	var out []Episode
	err := s.list("episodes", func(b []byte) error {
		var e Episode
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&e); err != nil {
			return err
		}
		if err := e.Validate(); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, err
}
func (s Store) ListPatterns() ([]Pattern, error) {
	var out []Pattern
	err := s.list("patterns", func(b []byte) error {
		var p Pattern
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	return out, err
}
func (s Store) ListProposals() ([]Proposal, error) {
	var out []Proposal
	err := s.list("proposals", func(b []byte) error {
		var p Proposal
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

func (s Store) ListExperiments() ([]Experiment, error) {
	var out []Experiment
	err := s.list("experiments", func(b []byte) error {
		var e Experiment
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&e); err != nil {
			return err
		}
		if err := e.Validate(); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

func (s Store) ListObservations() ([]Observation, error) {
	var out []Observation
	err := s.list("observations", func(b []byte) error {
		var o Observation
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return err
		}
		if err := o.Validate(); err != nil {
			return err
		}
		out = append(out, o)
		return nil
	})
	return out, err
}
