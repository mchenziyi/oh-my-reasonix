# Changelog

## [Unreleased]

## [v2.0.1] — 2026-08-04

### Added
- **Promotion Gate**：`omr evolve approve` 现在执行证据数量、质量评分、离线安全验证和内容 Hash 校验，并以稳定 JSON 结果报告门禁状态。
- 批准失败时不写入 Overlay、生成 Prompt 或 Manifest；通过门禁后仍保留快照和回滚路径。

### Compatibility
- v2.0.0 Proposal 仍可读取；缺失证据元数据的旧 Proposal 必须经过新的门禁并会被明确拒绝。

### Safety
- Promotion Gate 只证明协议完整性和安全约束，不宣称模型质量提升。

### Planned
- **v2.0.2 观察期指标**：记录 Proposal 生效前后 Episode、成本、失败分类和回滚原因。
- 设计文档：[`docs/OMR_V2_AUTONOMOUS_EVOLUTION_PLAN.zh-CN.md`](docs/OMR_V2_AUTONOMOUS_EVOLUTION_PLAN.zh-CN.md)。

### Added
- **T14: Comment Checker 运行时 Hook**：默认关闭的提交前注释质量门禁。新增 `omr hook comment-check enable/status/disable/guard` 命令，通过 Reasonix 原生 PreToolUse Hook 在 `git commit` 前自动调用 Comment Checker，存在 blocking finding 时以退出码 2 阻断提交。
- **internal/commenthook/**：纯合并器（EnableMerge/DisableMerge）、Guard stdin 解析器、事务写入（备份/原子写入/回滚）、路径安全（symlink 越界检测）、所有权标记、冲突检测、幂等保证。
- **Manifest HookRecord**：可选扩展字段，记录 Hook 启用状态、Entry SHA256 和文件 SHA256；向后兼容旧 Manifest。
- **omr doctor** 新增 `comment-hook` 诊断检查：支持 PASS/WARN/ERROR/UNSUPPORTED 四种状态，只诊断不写入。
- **Guard 退出码矩阵**：非提交命令/clean → 0，blocking finding → 2，扫描失败 fail closed → 2，非法 payload → 1。

### Fixed
- **Reasonix v1.18 machine interfaces**: project-scoped Session, Task, Hook, and Recovery queries now use `--project-root` and parse the published nested JSON schemas.
- **event token totals**: native `run_done.usage` is treated as the authoritative cumulative total, preventing duplicate counting with preceding `usage` events.
- **hook doctor**: detects the native Hook list/status interfaces instead of reporting them as unsupported.

### Completed
- **INT-06**: real-client verification completed against Reasonix v1.18.0. Session list/status/show, Task list, Hook list/status, Recovery, and native JSONL event parsing all passed.
- **T14**: Comment Checker 运行时 Hook 完成交付：配置模型、Guard、事务写入、CLI、Doctor 诊断、稳定绝对执行路径和旧版相对命令迁移；已通过 Reasonix v1.18 桌面端阻断、脱敏、放行与禁用验证。默认关闭，不自动启用。

---

## [v1.2.2] — 2026-07-29

### Added
- **omr comment-check**: offline comment quality checker CLI — 5 deterministic rules (R001-R005), JSON/human output, credential redaction, path safety, binary/large file skip (#T13)
- **model**: `internal/commentchecker/` — R001 (debug markers with allowlist), R002 (empty comments), R003 (comment-code similarity, warning only), R004 (credential leak, blocking, redacted), R005 (path safety)
- **tests**: 24 offline unit tests + 13 CLI end-to-end tests covering all rules, allowlist, credential redaction, project-root boundary, symlink traversal, binary/large skip, determinism, JSON schema, human output, snapshot consistency
- **docs**: README, TODO (T13 marked complete), gap matrix, CHANGELOG

### Known Issues
- **环境限制**：部分沙箱环境禁止 `httptest` 监听本地端口，可能导致 `internal/qualitybench` 全量测试失败；该失败不属于 Comment Checker 路径安全逻辑。
- **Hook/实时阻断**: v1.2.2 当时仅提供离线 CLI；该限制已在 Unreleased 的 T14 中解决。

---

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
