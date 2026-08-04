# Changelog

## [v2.0.9] — 2026-08-04

### Added
- **文档单一事实源清理（LP-06）**：README 中/英、CHANGELOG、Todo、Gap Matrix 与 Evolution 计划的版本和状态统一到 v2.0.9；22 份历史开发计划文档标注 Archived（保留审计价值），其中 Task Monitor 系列标注为依赖 Reasonix 官方接口（BLOCKED）。
- 新增 [当前可用能力矩阵](docs/OMR_CAPABILITY_MATRIX.zh-CN.md)：区分 CLI 已实现能力、Reasonix Desktop 可用范围与需要官方接口的范围，明确不把宿主未实现接口写成 OMR 已实现。
- 新增 [tests/docs_check.sh](tests/docs_check.sh)：文档链接检查、命令示例机器路径检查、过期表述扫描、中英文 README 状态一致性检查与能力矩阵存在性校验。

## [v2.0.8] — 2026-08-04

### Added
- **经验包签名与来源可信度（LP-05）**：经验包元数据扩展为 OMR 版本、创建工具版本、来源 Scope、Proposal 数量与签名算法；包格式升级为 `omr-evolution-proposals-v2`（v1 包仍可读取，视为未签名）。
- `omr evolve export --sign --key <path>`：使用本地 Ed25519 私钥签名（PKCS8 PEM 或原始密钥）；私钥只从用户指定位置读取，不写入项目或包，且拒绝 symlink 与 group/world 可读文件。
- `omr evolve import --require-signature [--trusted-key <pub.pem>]`：要求签名并可选绑定受信公钥；未签名包默认仍可 dry-run，实际导入时输出明确 warning，`--require-signature` 下拒绝。
- 签名覆盖元数据 + Proposal + 嵌入公钥的规范 payload（不含 sha256/signature 字段）；内容任意改动、密钥不匹配、未知字段、绝对路径与过大包均 fail closed；冲突批次零部分写入；导入 Proposal 保持 pending，不自动批准。

## [v2.0.7] — 2026-08-04

### Added
- **Comment Checker Hook 审计日志（LP-04）**：guard 每次决策（blocking / warning / 放行 / 解析失败）写入项目级 `.reasonix/omr/audit/audit.jsonl`，记录时间、事件、决策、规则计数、退出码、耗时与 OMR 版本。
- 新增 `omr hook comment-check logs --project-dir . --json` 查询（含按决策的摘要统计）；`--clear --dry-run` 零写入预览，`--clear` 幂等清除。
- 日志条目数与字节数上限（1000 条 / 256 KiB），超限按时间淘汰最旧；symlink、路径穿越与损坏日志 fail closed。
- 日志只保存脱敏统计，禁止命令正文、完整 toolArgs、文件内容与凭据；日志写入失败时 guard fail closed（阻断决策保持退出码 2，放行决策降级为显式失败），Doctor 新增 `comment-hook-audit` 检查明确报告日志不可用。

## [v2.0.6] — 2026-08-04

### Added
- **Profile/Prompt 效果基准（LP-03）**：新增 `omr benchmark profile`，用离线、可重复的 Fixture 比较 Profile/Prompt 的工程过程表现。
  - `omr benchmark profile --profile omr-explore --replay`：只评估指定 Profile 的 Fixture；
  - `omr benchmark profile --matrix --replay`：输出全部 Profile 的矩阵报告。
- **benchmarks/profile-quality/**：6 个 Fixture 覆盖 Explore 证据完整性、Planner 计划可执行性、Debug 根因与回归测试、Grill Me 假设隔离、Grill with Docs 原子写入、Comment Checker 安全阻断。
- 指标仅包括验收通过率、证据完整率、越界修改数、人工纠偏数、Token/成本与耗时；支持 Native/OMR 配对 Fixture（`native_replay`/`omr_replay`），报告保留失败证据、不挑选样本。
- 报告明确声明“process metrics only; not a model quality proof”；无需 API Key，Fake Provider/离线 Replay 即可完整运行。

## [v2.0.5] — 2026-08-04

### Added
- **Evolution 观测报告增强（LP-02）**：`omr evolve report --json` 新增按 Proposal 与 TaskClass 的聚合统计——before/after Episode 数量、成功率/失败率、Prompt/Output Token、耗时（时间跨度）、观察期进度（默认目标 5 个相关 Episode）与 `insufficient_evidence`/`observed`/`rolled_back` 状态。
- `omr evolve history <proposal-id> --json` 输出单个 Proposal 的详细观察历史：before/after Episode 摘要（脱敏，无 Session ID）、失败分类、Token 与回滚原因。
- 报告为稳定排序的确定性 JSON，快照测试保证字段形状稳定；报告只输出脱敏聚合统计，不输出 Overlay、Prompt、命令正文、Session ID 或模型输出，也不宣称“提升/显著改善”。

## [v2.0.4] — 2026-08-04

### Added
- **Evolution 数据保留与压缩（LP-01）**：`omr evolve doctor --json` 输出 Episode/Observation/Pattern/Proposal/Experiment 的只读统计（文件数、字节数、最早/最晚时间，损坏文件单独报告）。
- **`omr evolve prune`**：只删除已终态 Proposal（rejected/rolled_back）关联的旧 Episode/Observation，受 `--keep-episodes` 窗口约束；保留 pending/approved Proposal 及其必要证据；支持 `--dry-run` 零写入预览。
- **`omr evolve repair`**：只修复可确定推导的内容——孤儿 Observation、同 ID 重复文件（保留规范文件名）、无效 Pattern 索引；发现损坏 JSON、symlink 或跨 Scope 数据时 fail closed 零写入。
- **维护快照**：删除前写入带 Hash 的 `maintenance/snapshot-<hash>.json`（含 base64 内容），失败自动回滚；`RestoreSnapshot` 支持按 Hash 手动恢复，Doctor 报告快照列表。
- 所有维护操作保持 Scope 隔离、symlink 防护、0600 权限与原子写入。

## [Unreleased]

## [v2.0.3] — 2026-08-04

### Added
- **经验包导入导出**：`evolve export` 仅导出经过校验的 Proposal 元数据；`evolve import --dry-run` 支持预览、幂等跳过和冲突拒绝。
- 导入包限制大小与条目数，未知字段、Hash 不匹配、非法内容和冲突均 fail closed，失败时零写入。

## [v2.0.2] — 2026-08-04

### Added
- **观察期报告**：记录 Proposal 生效前后 Episode 的成功、失败分类、Token 与观察进度。
- `evolve report` 输出按 Proposal 的统计摘要；观察期不足 5 个相关 Episode 时标记 `insufficient_evidence`。
- `evolve history <proposal-id>` 支持按 Proposal 查询，Doctor 会校验观察记录。

## [v2.0.1] — 2026-08-04

### Added
- **Promotion Gate**：`omr evolve approve` 现在执行证据数量、质量评分、离线安全验证和内容 Hash 校验，并以稳定 JSON 结果报告门禁状态。
- 批准失败时不写入 Overlay、生成 Prompt 或 Manifest；通过门禁后仍保留快照和回滚路径。

### Compatibility
- v2.0.0 Proposal 仍可读取；缺失证据元数据的旧 Proposal 必须经过新的门禁并会被明确拒绝。

### Safety
- Promotion Gate 只证明协议完整性和安全约束，不宣称模型质量提升。

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
