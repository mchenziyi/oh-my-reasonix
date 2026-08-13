package memory

import (
	"context"
	"html"
	"sort"
	"strings"
	"time"
)

// BuildMemoryAuditWebExport renders derived state and governance metadata for
// one store. It is read-only and deterministic for the same explicit time.
func BuildMemoryAuditWebExport(ctx context.Context, store *FactStore, now time.Time) ([]byte, error) {
	if now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "audit export requires an explicit now timestamp")
	}
	scope := ScopeProject
	if store.storeScope == StoreScopeGlobal {
		scope = ScopeGlobal
	}
	derived, err := DeriveState(ctx, store, DerivedStateRequest{Scope: scope, Now: now})
	if err != nil {
		return nil, err
	}
	eventKeys, err := store.List(ctx, FactKindGovernanceEvent)
	if err != nil {
		return nil, err
	}
	sort.Strings(eventKeys)
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>OMR Mnemosyne Audit</title>")
	b.WriteString("<style>body{font:14px system-ui,sans-serif;margin:2rem;color:#222}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:.4rem;text-align:left}h1{font-size:1.4rem}.meta{color:#666}</style></head><body><h1>OMR Mnemosyne Audit</h1><p class=\"meta\">Scope: ")
	b.WriteString(html.EscapeString(string(scope)))
	b.WriteString("; evaluated: ")
	b.WriteString(html.EscapeString(now.UTC().Format(time.RFC3339)))
	b.WriteString("</p><p>Governance events: ")
	b.WriteString(itoa(len(eventKeys)))
	b.WriteString("</p><table><thead><tr><th>Memory</th><th>Type</th><th>Revision</th><th>Lifecycle</th><th>Health</th><th>Freshness</th><th>Usage</th></tr></thead><tbody>")
	for _, state := range derived.States {
		b.WriteString("<tr><td><code>")
		b.WriteString(html.EscapeString(state.MemoryID))
		b.WriteString("</code></td><td>")
		b.WriteString(html.EscapeString(string(state.MemoryType)))
		b.WriteString("</td><td>")
		b.WriteString(itoa(state.Revision))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(state.Lifecycle)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(state.Health)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(state.Freshness)))
		b.WriteString("</td><td>")
		b.WriteString(itoa(state.Usage.UsageCount))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table></body></html>\n")
	return []byte(b.String()), nil
}
