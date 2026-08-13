package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

// BuildMemoryManagerPage renders the local management page. It embeds only
// validated refs and escaped display text; all mutations go through the API.
func BuildMemoryManagerPage(ctx context.Context, store *FactStore, now time.Time) ([]byte, error) {
	if now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "manager page requires an explicit now timestamp")
	}
	keys, err := store.List(ctx, FactKindMemoryRevision)
	if err != nil {
		return nil, err
	}
	scope := ScopeProject
	if store.storeScope == StoreScopeGlobal {
		scope = ScopeGlobal
	}
	derived, err := DeriveState(ctx, store, DerivedStateRequest{Scope: scope, Now: now.UTC()})
	if err != nil {
		return nil, err
	}
	stateByKey := make(map[string]DerivedMemoryState, len(derived.States))
	for _, state := range derived.States {
		stateByKey[fmt.Sprintf("%s/%s/%s/%d", state.Scope, state.MemoryType, state.MemoryID, state.Revision)] = state
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
	sort.Slice(revisions, func(i, j int) bool { return revisionIdentityKey(revisions[i]) < revisionIdentityKey(revisions[j]) })
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"referrer\" content=\"no-referrer\"><title>OMR Mnemosyne Manager</title>")
	b.WriteString("<style>body{font:14px system-ui,sans-serif;margin:2rem;color:#222}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:.45rem;text-align:left}button{margin:.15rem;padding:.3rem .6rem}input{padding:.35rem;width:18rem}.meta{color:#666}</style></head><body>")
	b.WriteString("<h1>OMR Mnemosyne Manager</h1><p class=\"meta\">Local-only manual governance. Every action is revalidated and confirmed.</p><label>Reason <input id=\"reason\" maxlength=\"512\" value=\"manual review\"></label><label>Unfreeze basis refs (JSON array) <textarea id=\"basis-refs\" rows=\"2\" cols=\"60\" placeholder=\"[{&quot;kind&quot;:&quot;memory&quot;,...}]\"></textarea></label><table><thead><tr><th>Memory</th><th>Type</th><th>Revision</th><th>Lifecycle</th><th>Health</th><th>Usage</th><th>Title</th><th>Actions</th></tr></thead><tbody>")
	for _, rev := range revisions {
		action := WebManagementAction{SchemaVersion: SchemaVersion, ActionID: "web_ui_pending", Scope: rev.Scope, Target: memoryRefFromRevision(rev), Operation: "freeze", Reason: "manual review", RequestedAt: now.UTC().Format(time.RFC3339Nano)}
		data, err := json.Marshal(action)
		if err != nil {
			return nil, err
		}
		b.WriteString("<tr><td><code>")
		b.WriteString(html.EscapeString(rev.MemoryID))
		b.WriteString("</code></td><td>")
		b.WriteString(html.EscapeString(string(rev.MemoryType)))
		b.WriteString("</td><td>")
		b.WriteString(itoa(rev.Revision))
		state, ok := stateByKey[revisionIdentityKey(rev)]
		if !ok {
			return nil, storeError(CodeDerivedInvalidInput, "manager state is unavailable")
		}
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(state.Lifecycle)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(string(state.Health)))
		b.WriteString("</td><td>")
		b.WriteString(itoa(state.Usage.UsageCount))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(rev.Title))
		b.WriteString("</td><td>")
		b.WriteString("<button class=\"action\" data-operation=\"pin\" data-action='")
		b.WriteString(html.EscapeString(string(data)))
		b.WriteString("'>Pin</button><button class=\"action\" data-operation=\"unpin\" data-action='")
		b.WriteString(html.EscapeString(string(data)))
		b.WriteString("'>Unpin</button><button class=\"action\" data-operation=\"freeze\" data-action='")
		b.WriteString(html.EscapeString(string(data)))
		b.WriteString("'>Freeze</button><button class=\"action\" data-operation=\"archive\" data-action='")
		b.WriteString(html.EscapeString(string(data)))
		b.WriteString("'>Archive</button><button class=\"action\" data-operation=\"unfreeze\" data-action='")
		b.WriteString(html.EscapeString(string(data)))
		b.WriteString("'>Unfreeze</button></td></tr>")
	}
	b.WriteString("</tbody></table><pre id=\"result\"></pre><script>(function(){for(const el of document.querySelectorAll('.action')){el.onclick=async function(){const a=JSON.parse(el.dataset.action);a.action_id='web_ui_'+Date.now().toString(36);a.operation=el.dataset.operation;a.reason=document.getElementById('reason').value;if(a.operation==='unfreeze'){try{a.basis_refs=JSON.parse(document.getElementById('basis-refs').value);if(!Array.isArray(a.basis_refs)||a.basis_refs.length===0)throw new Error('basis refs required');}catch(e){document.getElementById('result').textContent='Unfreeze requires a valid non-empty basis refs JSON array';return;}}if(!window.confirm('Confirm governance action?'))return;const r=await fetch('/action/apply',{method:'POST',headers:{'Content-Type':'application/json','X-OMR-Confirm':'yes'},body:JSON.stringify(a)});document.getElementById('result').textContent=await r.text();if(r.ok)setTimeout(function(){location.reload();},150);};}})();</script></body></html>\n")
	return []byte(b.String()), nil
}
