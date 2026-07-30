# oh-my-reasonix

[中文版 README](README.md)

oh-my-reasonix (OMR) is a project-level engineering layer for [Reasonix](https://github.com/esengine/DeepSeek-Reasonix). It packages reusable prompts, read-only profiles, installation safeguards, quality benchmarks, configuration validation, and compatibility tooling for a Reasonix project.

OMR does not replace Reasonix. Reasonix remains responsible for model execution, sessions, tasks, permissions, sandboxing, hooks, todo state, and background jobs.

## Why OMR?

- Standardize project prompts and reusable Explore, Research, Debug, and Planner profiles.
- Install, upgrade, back up, roll back, and uninstall project assets safely.
- Validate configuration, manifests, hashes, profiles, and host compatibility.
- Import selected Claude configuration with dry-run, conflict, redaction, and rollback safeguards.
- Run deterministic quality fixtures and Native/OMR comparisons with cost and evidence metrics.

## Quick start

### Requirements

- Go 1.23+ when using `go run` or building from source;
- an installed and authenticated Reasonix client;
- a project containing `reasonix.toml`;
- Reasonix v1.17.20 or newer.

Use `omr version --json` to check the detected host and compatibility.

### Install into an existing project

Preview with `omr init --project-dir . --dry-run`, then run `omr init --project-dir .` and `omr doctor --project-dir .`.

OMR writes project-scoped artifacts such as `reasonix.toml`, `.reasonix/omr/generated/`, `.reasonix/skills/`, `.reasonix/omr/manifest.lock.yaml`, and `.reasonix/omr/backups/`. It does not start or take over the Reasonix desktop client.

### Build from source

Clone `https://github.com/mchenziyi/oh-my-reasonix.git`, run `go build -o omr ./cmd/omr`, then use `./omr init --project-dir /path/to/project --dry-run` followed by `./omr init --project-dir /path/to/project`.

## Profiles

OMR installs project-scoped profiles that use Reasonix's native subagent mechanism:

| Profile | Purpose | Write boundary |
|---|---|---|
| `omr-explore` | Explore code paths, call chains, and test entry points | read-only |
| `omr-research` | Research documentation, APIs, and external context | read-only |
| `omr-debug` | Identify root causes and minimal repair directions | read-only |
| `omr-planner` | Break work into stages, risks, and acceptance checks | read-only |
| `omr-frontend` | Inspect UI structure, interactions, and frontend tests | read-only |
| `omr-git` | Inspect history, diffs, and change impact | read-only |
| `omr-lsp` | Inspect symbols, references, and language diagnostics | read-only |
| `omr-grill-me` | Challenge goals, assumptions, boundaries, and acceptance criteria | read-only |
| `omr-grill-with-docs` | Persist confirmed decisions into `CONTEXT.md` and ADRs | writes after confirmation |

Reasonix built-in profiles remain native. List the effective profiles with `reasonix subagent list --dir .` and `omr profile list --project-dir . --json`.

For complex work, ask Reasonix to run `omr-grill-me` before planning. It is optional and does not force every task through a questionnaire.

## Upgrade and rollback

Use `omr upgrade --project-dir . --dry-run` before `omr upgrade --project-dir .`. Use `omr uninstall --project-dir . --dry-run` to preview removal. Modified OMR-owned files are detected and backups are preserved. OMR does not modify global PATH, API keys, or the Reasonix binary.

## Configuration and Claude compatibility

OMR discovers `.reasonix/omr/config.jsonc`, then `config.json`, then `config.toml`; the first existing file is used and files are not merged. Validate or migrate with `omr config validate --project-dir . --json`, `omr config schema --project-dir .`, and `omr config migrate --project-dir .`.

Selected `.claude/rules`, `skills`, `agents`, `commands`, MCP, and hook configuration can be imported with dry-run, conflict reporting, redaction, and rollback safeguards. Runtime hook semantics are reported when they cannot be preserved exactly.

## Comment checking

The offline checker is deterministic and does not call a model or network service. Run `omr comment-check --project-dir . --json`, optionally add `--path internal/foo.go` or `--allow-tags "TODO(admin),TODO(future)"`.

It checks temporary markers, empty comments, comment/code similarity, suspected credentials, and path safety. By default, paths are restricted to the project root; relative paths use `--project-dir`, and symlink escapes are rejected. Runtime hook enforcement requires a stable Reasonix hook interface and is intentionally not implemented in OMR.

## Quality benchmarks

Run `go run ./cmd/omr benchmark quality --replay --min-qualified-rate 1 --run-id nightly-20260730` for a deterministic replay. Reports include failure classification, retry/stall/review evidence, token and cost metrics, and paired Native/OMR comparisons. OMR does not claim model-quality improvements without paired evidence.

## Structured host interfaces

Where supported by the installed Reasonix version, OMR can forward read-only session, hook, task, recovery, and JSONL event queries. Examples include `omr session list --project-dir . --json`, `omr hook doctor --project-dir . --json`, `omr task list --project-dir . --json`, and `omr run --project-dir . --events-jsonl /tmp/reasonix-events.jsonl --json "run a task"`.

OMR does not read private Reasonix directories or infer session state from human-readable output.

## Current limitations

The following intentionally remain outside OMR until Reasonix exposes stable public interfaces: real-client INT-06 verification, runtime Comment Checker hooks, Subagent parent/child task trees, OMR-to-Desktop Task Monitor mapping, background-agent result aggregation, and Tmux/desktop live task panels.

## Development

Run `gofmt`, `git diff --check`, `go test ./...`, `go vet ./...`, and `go build ./...` before submitting changes. Some restricted environments prevent `httptest` from binding a local port; record that as an environment limitation rather than a product failure.

## License

MIT
