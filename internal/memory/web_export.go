package memory

import (
	"context"
	"html"
	"sort"
	"strings"
	"time"
)

// BuildMemoryWebExport renders a deterministic, read-only HTML view from the
// verified revision facts in store. It never reads or writes derived state.
func BuildMemoryWebExport(ctx context.Context, store *FactStore, now time.Time) ([]byte, error) {
	if now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "web export requires an explicit now timestamp")
	}
	keys, err := store.List(ctx, FactKindMemoryRevision)
	if err != nil {
		return nil, err
	}
	revisions := make([]MemoryRevision, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindMemoryRevision, key)
		if err != nil {
			return nil, err
		}
		rev, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		revisions = append(revisions, rev)
	}
	sort.Slice(revisions, func(i, j int) bool {
		return revisionIdentityKey(revisions[i]) < revisionIdentityKey(revisions[j])
	})

	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>OMR Mnemosyne</title>")
	b.WriteString("<style>body{font:14px system-ui,sans-serif;margin:2rem;color:#222}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:.4rem;text-align:left}h1{font-size:1.4rem}.meta{color:#666}code{font-family:ui-monospace,monospace}</style>")
	b.WriteString("</head><body><h1>OMR Mnemosyne Memory</h1><p class=\"meta\">Read-only derived view; ")
	b.WriteString(html.EscapeString(now.UTC().Format(time.RFC3339)))
	b.WriteString("</p><table><thead><tr><th>Scope</th><th>Type</th><th>Memory</th><th>Revision</th><th>Policy</th><th>Title</th><th>Summary</th><th>Relations</th></tr></thead><tbody>")
	for _, rev := range revisions {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(string(rev.Scope)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(rev.MemoryType)))
		b.WriteString("</td><td><code>")
		b.WriteString(html.EscapeString(rev.MemoryID))
		b.WriteString("</code></td><td>")
		b.WriteString(itoa(rev.Revision))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(rev.UsagePolicy)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(rev.Title))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(rev.Summary))
		b.WriteString("</td><td>")
		for i, rel := range rev.Relations {
			if i > 0 {
				b.WriteString("<br>")
			}
			b.WriteString(html.EscapeString(rel.Predicate))
			b.WriteString(" → <code>")
			b.WriteString(html.EscapeString(rel.Target.MemoryID))
			b.WriteString("@")
			b.WriteString(itoa(rel.Target.Revision))
			b.WriteString("</code>")
		}
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table></body></html>\n")
	return []byte(b.String()), nil
}
