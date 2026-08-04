package evolution

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

// --- Stats -----------------------------------------------------------------

// CollectionStats is a read-only summary of one evolution collection.
type CollectionStats struct {
	Name         string   `json:"name"`
	Files        int      `json:"files"`
	Bytes        int64    `json:"bytes"`
	EarliestTime string   `json:"earliest_time,omitempty"`
	LatestTime   string   `json:"latest_time,omitempty"`
	Damaged      []string `json:"damaged,omitempty"`
}

// StoreStats reports file counts, byte totals, and created_at time ranges
// for every evolution collection. It is read-only and never writes.
type StoreStats struct {
	SchemaVersion int               `json:"schema_version"`
	ScopeID       string            `json:"scope_id"`
	Collections   []CollectionStats `json:"collections"`
	Snapshots     []SnapshotInfo    `json:"snapshots,omitempty"`
}

type SnapshotInfo struct {
	Hash      string `json:"hash"`
	Operation string `json:"operation"`
	CreatedAt string `json:"created_at"`
	Entries   int    `json:"entries"`
}

var collectionDirs = []string{"episodes", "observations", "patterns", "proposals", "experiments"}

// Stats computes read-only collection statistics. Corrupt files are reported
// in Damaged instead of failing the whole summary; destructive operations
// still fail closed through their own scan.
func (s Store) Stats() (StoreStats, error) {
	if err := s.checkScope(); err != nil {
		return StoreStats{}, err
	}
	stats := StoreStats{SchemaVersion: SchemaVersion, ScopeID: s.ScopeID, Collections: []CollectionStats{}}
	for _, dir := range collectionDirs {
		cs, err := s.collectionStats(dir)
		if err != nil {
			return StoreStats{}, err
		}
		stats.Collections = append(stats.Collections, cs)
	}
	snaps, err := s.listSnapshots()
	if err != nil {
		return StoreStats{}, err
	}
	stats.Snapshots = snaps
	return stats, nil
}

func (s Store) collectionStats(dir string) (CollectionStats, error) {
	cs := CollectionStats{Name: dir}
	p, err := s.safeJoin(dir)
	if err != nil {
		return cs, err
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return cs, nil
	}
	if err != nil {
		return cs, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(p, e.Name())
		if info, lerr := os.Lstat(full); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return cs, fmt.Errorf("symlink path rejected: %s", full)
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return cs, err
		}
		cs.Files++
		cs.Bytes += int64(len(b))
		var rec struct {
			CreatedAt string `json:"created_at"`
		}
		if err := json.Unmarshal(b, &rec); err != nil || rec.CreatedAt == "" {
			cs.Damaged = append(cs.Damaged, e.Name())
			continue
		}
		if cs.EarliestTime == "" || rec.CreatedAt < cs.EarliestTime {
			cs.EarliestTime = rec.CreatedAt
		}
		if rec.CreatedAt > cs.LatestTime {
			cs.LatestTime = rec.CreatedAt
		}
	}
	return cs, nil
}

// --- Snapshot --------------------------------------------------------------

// SnapshotEntry records one removed file so it can be restored exactly.
type SnapshotEntry struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	ContentB64 string `json:"content_b64,omitempty"`
}

// Snapshot records files removed by a prune or repair operation. The file's
// own SHA256 covers the JSON without the self-referential sha256 field.
type Snapshot struct {
	SchemaVersion int             `json:"schema_version"`
	ScopeID       string          `json:"scope_id"`
	Operation     string          `json:"operation"`
	CreatedAt     string          `json:"created_at"`
	Entries       []SnapshotEntry `json:"entries"`
	SHA256        string          `json:"sha256"`
}

func (sn Snapshot) hash() string {
	sn.SHA256 = ""
	b, _ := json.Marshal(sn)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (sn Snapshot) validate() error {
	if sn.SchemaVersion != SchemaVersion || sn.ScopeID == "" || sn.Operation == "" || sn.CreatedAt == "" {
		return fmt.Errorf("invalid snapshot")
	}
	if sn.SHA256 == "" || sn.hash() != sn.SHA256 {
		return fmt.Errorf("snapshot hash mismatch")
	}
	return nil
}

func snapshotDir(root string) string { return filepath.Join(root, "maintenance") }

func (s Store) snapshotPath(hash string) (string, error) {
	return s.safeJoin(filepath.Join("maintenance", "snapshot-"+hash+".json"))
}

// writeSnapshot persists the snapshot and returns its content hash.
func (s Store) writeSnapshot(sn Snapshot) (string, error) {
	sn.SchemaVersion = SchemaVersion
	sn.ScopeID = s.ScopeID
	sn.CreatedAt = Now()
	sn.SHA256 = sn.hash()
	b, err := json.MarshalIndent(sn, "", "  ")
	if err != nil {
		return "", err
	}
	p, err := s.snapshotPath(sn.SHA256)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return "", err
	}
	b = append(b, '\n')
	if err := fileutil.AtomicWrite(p, b, 0600); err != nil {
		return "", err
	}
	return sn.SHA256, nil
}

func (s Store) listSnapshots() ([]SnapshotInfo, error) {
	if err := s.checkScope(); err != nil {
		return nil, err
	}
	p, err := s.safeJoin("maintenance")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SnapshotInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "snapshot-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		hash := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "snapshot-"), ".json")
		b, err := os.ReadFile(filepath.Join(p, e.Name()))
		if err != nil {
			return nil, err
		}
		var sn Snapshot
		if err := json.Unmarshal(b, &sn); err != nil || sn.SchemaVersion != SchemaVersion {
			continue // skip damaged snapshots; do not block read-only stats
		}
		out = append(out, SnapshotInfo{Hash: hash, Operation: sn.Operation, CreatedAt: sn.CreatedAt, Entries: len(sn.Entries)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

// RestoreSnapshot restores every file recorded in the snapshot with the
// given hash. Returns the number of files restored.
func (s Store) RestoreSnapshot(hash string) (int, error) {
	if err := s.checkScope(); err != nil {
		return 0, err
	}
	p, err := s.snapshotPath(hash)
	if err != nil {
		return 0, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, err
	}
	var sn Snapshot
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sn); err != nil {
		return 0, fmt.Errorf("invalid snapshot: %w", err)
	}
	if err := sn.validate(); err != nil {
		return 0, err
	}
	restored := 0
	for _, entry := range sn.Entries {
		if strings.Contains(entry.Path, "..") || strings.HasPrefix(entry.Path, "/") {
			return restored, fmt.Errorf("snapshot contains unsafe path: %s", entry.Path)
		}
		content, err := base64.StdEncoding.DecodeString(entry.ContentB64)
		if err != nil {
			return restored, fmt.Errorf("snapshot entry corrupt: %w", err)
		}
		got := sha256.Sum256(content)
		if hex.EncodeToString(got[:]) != entry.SHA256 {
			return restored, fmt.Errorf("snapshot entry hash mismatch: %s", entry.Path)
		}
		dst, err := s.safeJoin(strings.Split(entry.Path, string(filepath.Separator))...)
		if err != nil {
			return restored, err
		}
		if err := fileutil.AtomicWrite(dst, content, 0600); err != nil {
			return restored, err
		}
		restored++
	}
	return restored, nil
}

// --- Prune -----------------------------------------------------------------

// Terminal proposal statuses. Evidence attached to these may be pruned.
func isTerminalStatus(status string) bool {
	return status == "rejected" || status == "rolled_back"
}

type PruneOptions struct {
	KeepEpisodes int  // keep at most this many newest episodes; 0 keeps none beyond terminal evidence
	DryRun       bool // preview without writing
}

type PruneResult struct {
	SchemaVersion       int      `json:"schema_version"`
	DryRun              bool     `json:"dry_run"`
	EpisodesRemoved     int      `json:"episodes_removed"`
	ObservationsRemoved int      `json:"observations_removed"`
	KeepEpisodes        int      `json:"keep_episodes"`
	Snapshot            string   `json:"snapshot,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

// checkNoSymlinks fails closed when any collection or snapshot directory
// contains a symlink.
func (s Store) checkNoSymlinks() error {
	dirs := append(append([]string(nil), collectionDirs...), "maintenance")
	for _, dir := range dirs {
		p, err := s.safeJoin(dir)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			full := filepath.Join(p, e.Name())
			if info, lerr := os.Lstat(full); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink path rejected: %s", full)
			}
		}
	}
	return nil
}

func (s Store) Prune(opts PruneOptions) (PruneResult, error) {
	result := PruneResult{SchemaVersion: SchemaVersion, DryRun: opts.DryRun, KeepEpisodes: opts.KeepEpisodes}
	if opts.KeepEpisodes < 0 {
		return result, fmt.Errorf("keep-episodes must be >= 0")
	}
	if err := s.checkScope(); err != nil {
		return result, err
	}
	if err := s.checkNoSymlinks(); err != nil {
		return result, err
	}
	episodes, err := s.ListEpisodes()
	if err != nil {
		return result, err
	}
	proposals, err := s.ListProposals()
	if err != nil {
		return result, err
	}
	observations, err := s.ListObservations()
	if err != nil {
		return result, err
	}
	patterns, err := s.ListPatterns()
	if err != nil {
		return result, err
	}

	active := map[string]bool{}   // proposals that must keep evidence
	terminal := map[string]bool{} // proposals whose evidence may be pruned
	for _, p := range proposals {
		if isTerminalStatus(p.Status) {
			terminal[p.ID] = true
		} else {
			active[p.ID] = true
		}
	}

	// Episode IDs referenced by active proposals (through patterns or observations).
	protected := map[string]bool{}
	patternByID := map[string]Pattern{}
	for _, pat := range patterns {
		patternByID[pat.ID] = pat
	}
	for _, p := range proposals {
		if !active[p.ID] {
			continue
		}
		if pat, ok := patternByID[p.PatternID]; ok {
			for _, id := range pat.EpisodeIDs {
				protected[id] = true
			}
		}
	}
	for _, o := range observations {
		if active[o.ProposalID] {
			protected[o.EpisodeID] = true
		}
	}

	// Episode IDs connected to terminal proposals (through patterns or observations).
	terminalLinked := map[string]bool{}
	for _, p := range proposals {
		if !terminal[p.ID] {
			continue
		}
		if pat, ok := patternByID[p.PatternID]; ok {
			for _, id := range pat.EpisodeIDs {
				terminalLinked[id] = true
			}
		}
	}
	for _, o := range observations {
		if terminal[o.ProposalID] {
			terminalLinked[o.EpisodeID] = true
		}
	}

	// Keep window: the newest KeepEpisodes episodes by CreatedAt.
	keepWindow := map[string]bool{}
	sorted := append([]Episode(nil), episodes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt > sorted[j].CreatedAt })
	for i := 0; i < opts.KeepEpisodes && i < len(sorted); i++ {
		keepWindow[sorted[i].ID] = true
	}

	toRemove := map[string]SnapshotEntry{} // relative path -> entry
	removeObservation := func(o Observation) {
		rel := filepath.Join("observations", o.ID+".json")
		if _, dup := toRemove[rel]; dup {
			return
		}
		b, err := os.ReadFile(filepath.Join(s.Root, rel))
		if err != nil {
			return
		}
		sum := sha256.Sum256(b)
		toRemove[rel] = SnapshotEntry{Path: rel, SHA256: hex.EncodeToString(sum[:]), ContentB64: base64.StdEncoding.EncodeToString(b)}
	}
	removeEpisode := func(e Episode) {
		rel := filepath.Join("episodes", e.ID+".json")
		if _, dup := toRemove[rel]; dup {
			return
		}
		b, err := os.ReadFile(filepath.Join(s.Root, rel))
		if err != nil {
			return
		}
		sum := sha256.Sum256(b)
		toRemove[rel] = SnapshotEntry{Path: rel, SHA256: hex.EncodeToString(sum[:]), ContentB64: base64.StdEncoding.EncodeToString(b)}
	}

	for _, e := range episodes {
		// Only old episodes linked to terminal evidence are pruned; protected
		// and in-window episodes are kept.
		if terminalLinked[e.ID] && !protected[e.ID] && !keepWindow[e.ID] {
			removeEpisode(e)
		}
	}
	for _, o := range observations {
		if terminal[o.ProposalID] {
			removeObservation(o)
		}
	}

	if len(toRemove) == 0 {
		return result, nil
	}

	sn := Snapshot{Operation: "prune", Entries: []SnapshotEntry{}}
	for _, rel := range sortedKeys(toRemove) {
		sn.Entries = append(sn.Entries, toRemove[rel])
	}
	if opts.DryRun {
		result.EpisodesRemoved = countRemoved(toRemove, "episodes")
		result.ObservationsRemoved = countRemoved(toRemove, "observations")
		return result, nil
	}
	hash, err := s.writeSnapshot(sn)
	if err != nil {
		return result, err
	}
	result.Snapshot = hash
	if err := s.removeWithRollback(sn); err != nil {
		return result, err
	}
	result.EpisodesRemoved = countRemoved(toRemove, "episodes")
	result.ObservationsRemoved = countRemoved(toRemove, "observations")
	return result, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func countRemoved(m map[string]SnapshotEntry, prefix string) int {
	n := 0
	for rel := range m {
		if strings.HasPrefix(rel, prefix+"/") {
			n++
		}
	}
	return n
}

// removeWithRollback removes all snapshot entries; on any failure it restores
// already-removed files from the snapshot content before returning the error.
func (s Store) removeWithRollback(sn Snapshot) error {
	removed := 0
	for _, entry := range sn.Entries {
		p, err := s.safeJoin(strings.Split(entry.Path, string(filepath.Separator))...)
		if err != nil {
			s.restoreEntries(sn.Entries[:removed])
			return err
		}
		if info, lerr := os.Lstat(p); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			s.restoreEntries(sn.Entries[:removed])
			return fmt.Errorf("symlink path rejected: %s", p)
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			s.restoreEntries(sn.Entries[:removed])
			return err
		}
		removed++
	}
	return nil
}

func (s Store) restoreEntries(entries []SnapshotEntry) {
	for _, entry := range entries {
		content, err := base64.StdEncoding.DecodeString(entry.ContentB64)
		if err != nil {
			continue
		}
		dst, err := s.safeJoin(strings.Split(entry.Path, string(filepath.Separator))...)
		if err != nil {
			continue
		}
		_ = fileutil.AtomicWrite(dst, content, 0600)
	}
}

// --- Repair ----------------------------------------------------------------

type RepairOptions struct {
	DryRun bool
}

type RepairResult struct {
	SchemaVersion int      `json:"schema_version"`
	DryRun        bool     `json:"dry_run"`
	FilesRemoved  int      `json:"files_removed"`
	IndexesFixed  int      `json:"indexes_fixed"`
	Unresolved    []string `json:"unresolved,omitempty"`
	Snapshot      string   `json:"snapshot,omitempty"`
}

// scanCollection reads every JSON file in a collection directory. It rejects
// symlinks, returns file contents keyed by file name, and collects the
// canonical record ID parsed from each file.
func (s Store) scanCollection(dir string) (map[string][]byte, map[string]string, error) {
	contents := map[string][]byte{}
	idByName := map[string]string{}
	p, err := s.safeJoin(dir)
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return contents, idByName, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(p, e.Name())
		if info, lerr := os.Lstat(full); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("symlink path rejected: %s", full)
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return nil, nil, err
		}
		var rec struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(b, &rec); err != nil || rec.ID == "" {
			return nil, nil, fmt.Errorf("unresolvable JSON in %s/%s", dir, e.Name())
		}
		contents[e.Name()] = b
		idByName[e.Name()] = rec.ID
	}
	return contents, idByName, nil
}

func (s Store) Repair(opts RepairOptions) (RepairResult, error) {
	result := RepairResult{SchemaVersion: SchemaVersion, DryRun: opts.DryRun}
	if err := s.checkScope(); err != nil {
		return result, err
	}

	// Fail closed on corrupt JSON or symlinks before planning any fix.
	episodes, epByID, err := s.scanCollection("episodes")
	if err != nil {
		result.Unresolved = append(result.Unresolved, err.Error())
		return result, err
	}
	observations, _, err := s.scanCollection("observations")
	if err != nil {
		result.Unresolved = append(result.Unresolved, err.Error())
		return result, err
	}
	patterns, patByID, err := s.scanCollection("patterns")
	if err != nil {
		result.Unresolved = append(result.Unresolved, err.Error())
		return result, err
	}
	proposals, _, err := s.scanCollection("proposals")
	if err != nil {
		result.Unresolved = append(result.Unresolved, err.Error())
		return result, err
	}

	// Plans: relative path -> new content (nil means remove).
	type fix struct {
		content []byte // nil = delete file
		indexed bool   // true = rewrite pattern index
	}
	plan := map[string]*fix{}

	// 1. Orphan observations: referenced proposal or episode does not exist.
	for name, b := range observations {
		var o Observation
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return result, fmt.Errorf("unresolvable observation %s", name)
		}
		_, proposalExists := proposals[o.ProposalID+".json"]
		_, episodeExists := episodes[o.EpisodeID+".json"]
		if !proposalExists || !episodeExists {
			plan[filepath.Join("observations", name)] = nil
		}
	}

	// 2. Duplicate files: several file names carrying the same record ID.
	//    Keep the lexicographically first name, remove the rest.
	duplicatePlans := map[string]string{} // duplicate file -> canonical file
	collectDuplicates := func(idByName map[string]string, dir string) {
		byID := map[string][]string{}
		for name, id := range idByName {
			byID[id] = append(byID[id], name)
		}
		for id, names := range byID {
			if len(names) < 2 {
				continue
			}
			// Prefer the canonical <id>.json name as the survivor.
			canonical := ""
			for _, n := range names {
				if n == id+".json" {
					canonical = n
					break
				}
			}
			if canonical == "" {
				sort.Strings(names)
				canonical = names[0]
			}
			for _, dup := range names {
				if dup == canonical {
					continue
				}
				duplicatePlans[filepath.Join(dir, dup)] = filepath.Join(dir, canonical)
			}
		}
	}
	obsIDByName := map[string]string{}
	for name, b := range observations {
		var o Observation
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return result, fmt.Errorf("unresolvable observation %s", name)
		}
		obsIDByName[name] = o.ID
	}
	collectDuplicates(epByID, "episodes")
	collectDuplicates(obsIDByName, "observations")
	collectDuplicates(patByID, "patterns")

	// 3. Invalid pattern indexes: episode IDs that do not exist.
	var patternsList []Pattern
	for name, b := range patterns {
		var p Pattern
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return result, fmt.Errorf("unresolvable pattern %s", name)
		}
		patternsList = append(patternsList, p)
	}
	// pattern id -> file name
	patternFile := map[string]string{}
	for name, id := range patByID {
		patternFile[id] = name
	}
	for _, p := range patternsList {
		valid := make([]string, 0, len(p.EpisodeIDs))
		dropped := false
		for _, id := range p.EpisodeIDs {
			if _, ok := episodes[id+".json"]; ok {
				valid = append(valid, id)
			} else {
				dropped = true
			}
		}
		if dropped {
			fixed := p
			fixed.EpisodeIDs = valid
			b, err := Encode(fixed)
			if err != nil {
				return result, err
			}
			b = append(b, '\n')
			if name, ok := patternFile[p.ID]; ok {
				plan[filepath.Join("patterns", name)] = &fix{content: b, indexed: true}
			}
		}
	}

	// Merge duplicate removals (they are file deletions, not rewrites).
	for rel := range duplicatePlans {
		if _, exists := plan[rel]; !exists {
			plan[rel] = nil
		}
	}

	if len(plan) == 0 {
		return result, nil
	}

	// Snapshot everything that will change so repair is reversible.
	sn := Snapshot{Operation: "repair", Entries: []SnapshotEntry{}}
	for _, rel := range sortedKeys(plan) {
		b, err := os.ReadFile(filepath.Join(s.Root, rel))
		if err != nil {
			return result, err
		}
		sum := sha256.Sum256(b)
		sn.Entries = append(sn.Entries, SnapshotEntry{Path: rel, SHA256: hex.EncodeToString(sum[:]), ContentB64: base64.StdEncoding.EncodeToString(b)})
	}
	if opts.DryRun {
		for rel := range plan {
			if plan[rel] == nil {
				result.FilesRemoved++
			} else if plan[rel].indexed {
				result.IndexesFixed++
			}
		}
		return result, nil
	}

	hash, err := s.writeSnapshot(sn)
	if err != nil {
		return result, err
	}
	result.Snapshot = hash

	// Apply: deletes first, then rewrites.
	for _, rel := range sortedKeys(plan) {
		if plan[rel] != nil {
			continue
		}
		p, err := s.safeJoin(strings.Split(rel, string(filepath.Separator))...)
		if err != nil {
			s.restoreEntries(sn.Entries)
			return result, err
		}
		if info, lerr := os.Lstat(p); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			s.restoreEntries(sn.Entries)
			return result, fmt.Errorf("symlink path rejected: %s", p)
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			s.restoreEntries(sn.Entries)
			return result, err
		}
		result.FilesRemoved++
	}
	for _, rel := range sortedKeys(plan) {
		f := plan[rel]
		if f == nil || !f.indexed {
			continue
		}
		p, err := s.safeJoin(strings.Split(rel, string(filepath.Separator))...)
		if err != nil {
			s.restoreEntries(sn.Entries)
			return result, err
		}
		if err := fileutil.AtomicWrite(p, f.content, 0600); err != nil {
			s.restoreEntries(sn.Entries)
			return result, err
		}
		result.IndexesFixed++
	}
	return result, nil
}
