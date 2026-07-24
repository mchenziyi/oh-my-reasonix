# Changelog

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
