package evolution

import "sort"

// DetectPattern returns a deterministic pattern when three distinct failed
// episodes share the same task and failure class. Existing patterns are used
// by the caller to make this trigger one-shot.
func DetectPattern(episodes []Episode, threshold int) *Pattern {
	if threshold < 1 {
		threshold = 3
	}
	groups := map[string][]Episode{}
	for _, e := range episodes {
		if e.Succeeded || e.FailureClass == "" {
			continue
		}
		key := e.TaskClass + "\x00" + e.FailureClass
		groups[key] = append(groups[key], e)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		es := groups[key]
		if len(es) < threshold {
			continue
		}
		sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
		ids := make([]string, threshold)
		for i := range ids {
			ids[i] = es[i].ID
		}
		return &Pattern{SchemaVersion: SchemaVersion, ID: NewID("pattern", key), TaskClass: es[0].TaskClass, FailureClass: es[0].FailureClass, EpisodeIDs: ids, CreatedAt: Now()}
	}
	return nil
}
