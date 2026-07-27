# Changelog

## [v1.2.1] — 2026-07-27

### Added
- **omr-grill-with-docs**: confirmed-facts-to-docs profile — dry-run preview, user confirmation, atomic writes to CONTEXT.md and ADR files (#T12)
- **assets**: embedded SKILL.md, embed.go/assets.go/paths.go/install.go registration, manifest.yaml v1.2.1
- **model**: `internal/grillwithdocs/` — Plan/Apply functions with conflict detection, idempotency, ADR numbering, and 6 security checks (abs path, path traversal, symlink escape, .reasonix, outside-project-root, unconfirmed assumptions isolation)
- **tests**: 11 offline replay tests covering dry-run, confirmed write, ADR numbering, idempotency, conflict detection, unconfirmed isolation, and 5 path-security scenarios
- **docs**: README profile list, TODO (T12 marked complete), CHANGELOG

### Changed
- **version**: `assets/manifest.yaml` 1.2.0 → 1.2.1

### Known Issues
- **INT-06**: real-client verification pending — requires Reasonix public machine interface stable release

---



### Added
- **omr-grill-me**: read-only challenge profile that discovers goal ambiguity, unconfirmed assumptions, edge cases, failure paths, and acceptance gaps before complex development tasks (#T11)
- **assets**: embedded SKILL.md, embed.go, assets.go/paths.go/install.go registration, manifest.yaml v1.2.0
- **tests**: 5 Prompt 契约测试（验证 SKILL.md 声明了停止条件和只读约束）+ 5 离线回放测试（纯 Go 数据结构模拟质询轮次、停止条件、假设隔离和文件快照不变）
- **docs**: README profile list, gap matrix (challenge agent row), TODO (T11 marked complete), CHANGELOG

### Changed
- **version**: `assets/manifest.yaml` 1.1.3 → 1.2.0
- **validate**: `cmd/omr/main.go` `knownProfiles` now includes `omr-git`, `omr-lsp`, `omr-grill-me` — prevents false "routes to unknown profile" warnings

### Known Issues
- **INT-06**: real-client verification pending — requires Reasonix public machine interface stable release

---

## [v1.1.3] — 2026-07-24

### Added
- **config validate**: missing config no longer errors — returns `valid:true configured:false` with exit 0 (#bd23e39)
- **config validate**: JSON output now includes `configured` field to distinguish unconfigured / valid / invalid states
- **README**: one-minute install section, v1.17.20 machine interface compatibility table, common errors & troubleshooting (#08e3078)
- **README**: install/upgrade/backup/rollback/uninstall command examples
- **tests**: +11 regression tests for v1.17.20 machine interfaces (SessionRecovery, HookStatus, event schema, sequence, sanitization) (#88930b9)
- **fixtures**: +2 offline quality fixtures (event-stream-failure, failed-event-persistence) (#0c57bd4)
- **docs**: autonomous 2-day execution report (#bb80844)

### Changed
- **CLI**: merged duplicate `writeJSONReport`/`writeJSONValue` into single function with `label` parameter (#06bb94b)
- **version**: synced `main.go` version var and `INSTALL.md` references from v1.1.1 to v1.1.2 (#49bde3e)

### Fixed
- **hookDirArgs**: pass `--dir` instead of `--project-root` to Reasonix (#4adf65e)
- **doctor**: v1.17.20 integration completed (#659af3d)

### Known Issues
- **INT-06**: real-client verification pending — requires Reasonix public machine interface stable release

---

## [v1.1.2] — 2026-07-21

- Docs: archive OMR A/B testing plans and reports
- Multiple documentation improvements

## [v1.1.1] — 2026-07-18

- Initial public release
- Core install/upgrade/uninstall workflow
- Built-in OMR profiles (explore, research, debug, planner, frontend, git, lsp)
- Claude configuration import (rules, skills, agents, commands, MCP, hooks)
- Quality benchmark system with offline fixture replay
- Reasonix machine interface: session, hook, task read-only queries
- Config validate, schema, and migrate commands
- TOML/JSONC/JSON configuration support
- Doctor diagnostics
- Cache guard for deterministic replay
