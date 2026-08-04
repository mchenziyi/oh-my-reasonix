# OMR v2.0.4～v2.0.9 本地增强开发任务书

> 用途：交给 Reasonix Agent 在 `oh-my-reasonix` 仓库内执行。
> 
> 目标：在不等待 Reasonix 官方接口的前提下，继续提升 OMR 的长期运行、可观测性、质量证明、安全审计和文档一致性。
> 
> 当前基线：OMR v2.0.3；T01～T14、INT-01～INT-06、EV-MVP-00～10、EV-POST-01～03 已完成。

## 总体约束

- 只修改 OMR 仓库，不修改 Reasonix 源码、二进制、全局 PATH、API Key 或正式项目。
- 使用临时目录和 Fake Reasonix，不调用真实模型，不读取用户正式项目。
- 每个阶段先写失败测试，再做最小实现；阶段之间独立提交。
- 不伪造桌面端任务完成事件、Subagent 父子关系或 Task Monitor 数据。
- 所有新 JSON 结构使用 `schema_version`，未知字段默认拒绝。
- 所有报告只输出脱敏统计，不输出 Prompt、命令正文、思考内容或凭据。

## 阶段总览

| 阶段 | 版本 | 工作项 | 依赖 | 优先级 |
|---|---|---|---|---|
| LP-01 | v2.0.4 | Evolution 数据保留、压缩与修复 | 无 | P1 |
| LP-02 | v2.0.5 | Evolution 观测报告增强 | LP-01 | P1 |
| LP-03 | v2.0.6 | Profile/Prompt 效果基准 | 无 | P1 |
| LP-04 | v2.0.7 | Hook 日志与审计报告 | 无 | P2 |
| LP-05 | v2.0.8 | 经验包签名与来源可信度 | 无 | P2 |
| LP-06 | v2.0.9 | 文档、Todo 和差距矩阵清理 | LP-01～05 | P2 |

每个阶段完成后：`gofmt`、`git diff --check`、`go test -count=1 ./...`、`go vet ./...`、`go build ./cmd/omr`，通过后推送 `main` 并创建对应 Tag。不得覆盖已有 Tag。

---

## LP-01：Evolution 数据保留、压缩与修复（v2.0.4）

### 目标

防止 `.reasonix/omr/evolution/` 无限增长，并让损坏、孤儿和重复记录可被 Doctor 识别和安全修复。

### 实现范围

- 新增只读统计：Episode、Observation、Pattern、Proposal、Experiment 文件数量、字节数和最早/最晚时间。
- 新增显式命令：

```text
omr evolve doctor --json
omr evolve prune --dry-run --keep-episodes 500 --project-dir . --json
omr evolve prune --keep-episodes 500 --project-dir . --json
omr evolve repair --dry-run --project-dir . --json
omr evolve repair --project-dir . --json
```

- `prune` 只删除已终态 Proposal 关联的旧 Episode/Observation；默认 dry-run。
- `repair` 只修复可确定推导的内容：孤儿 Observation、重复文件、无效索引；无法确定时报告并拒绝写入。
- 删除前创建带 Hash 的快照；失败自动恢复。
- 保持 Scope 隔离、symlink 防护、0600 文件权限和原子写入。

### 验收

- dry-run 零写入；重复 prune/repair 幂等。
- 损坏 JSON、路径穿越、symlink、跨 Scope 数据均 fail closed。
- 快照可恢复，Doctor 能发现修复前后的差异。
- 不删除 pending/approved Proposal 或其必要证据。

---

## LP-02：Evolution 观测报告增强（v2.0.5）

### 目标

让用户能判断某个 Proposal 的观察数据是否足够，但不输出模型质量或因果结论。

### 实现范围

- 扩展 `omr evolve report`：
  - 按 Proposal、TaskClass、FailureClass 聚合；
  - before/after Episode 数量、成功率、失败率、Token、耗时；
  - 当前观察期进度（默认 5 个相关 Episode）；
  - 自动回滚原因和时间；
  - `insufficient_evidence`、`observed`、`rolled_back` 状态。
- 增加 `omr evolve history <proposal-id> --json` 的详细统计输出。
- 提供稳定排序和报告快照测试。
- 不输出完整 Overlay、Prompt、命令、Session ID 或模型输出。

### 验收

- before/after 归属稳定，重复 Episode 不重复计数。
- 不同项目 Scope 的报告完全隔离。
- 观察不足时明确报告 `insufficient_evidence`。
- 失败分类、Token 和回滚原因统计可复现。
- 无完整配对数据时不得输出“提升”“显著改善”等结论。

---

## LP-03：Profile/Prompt 效果基准（v2.0.6）

### 目标

用离线、可重复的 Fixture 比较不同 Profile/Prompt 的工程过程表现，证明治理指标变化，不宣称模型能力提升。

### 实现范围

- 新增 `benchmarks/profile-quality/` Fixture，至少覆盖：
  - Explore 证据完整性；
  - Planner 计划可执行性；
  - Debug 根因与回归测试；
  - Grill Me 假设隔离；
  - Grill with Docs 原子写入；
  - Comment Checker 安全阻断。
- 新增命令：

```text
omr benchmark profile --profile omr-explore --replay --json
omr benchmark profile --matrix --json
```

- 指标仅包括：验收通过率、证据完整率、越界修改数、人工纠偏数、Token/成本、耗时。
- 支持 Native/OMR 配对 Fixture，但报告必须保留失败证据，不得挑选样本。

### 验收

- 同一 Fixture 重放结果确定性。
- OMR/Native 使用同一任务模板和验收条件。
- 失败、环境限制和模型失败分类清晰。
- 无 API Key 也能用 Fake Provider 完成测试。
- 报告明确“不等于模型质量证明”。

---

## LP-04：Hook 日志与审计报告（v2.0.7）

### 目标

补足 Comment Checker Hook 的本地审计可见性，便于排查误阻断、Hook 不可执行和环境 PATH 问题。

### 实现范围

- 在项目级 `.reasonix/omr/audit/` 保存脱敏 Hook 结果：时间、事件、决策、规则计数、退出码、耗时、OMR 版本。
- 新增命令：

```text
omr hook comment-check logs --project-dir . --json
omr hook comment-check logs --project-dir . --clear --dry-run
omr hook comment-check logs --project-dir . --clear
```

- 日志大小和条目数上限；超过上限按时间淘汰。
- 禁止保存命令正文、完整 toolArgs、文件内容和凭据。
- 日志写入失败不得让非阻断 Hook 伪造成功；Doctor 明确报告日志不可用。

### 验收

- blocking、warning、放行、解析失败四类日志均可查询。
- 敏感字段脱敏测试通过。
- clear dry-run 零写入，clear 幂等。
- symlink、路径穿越和损坏日志被拒绝。

---

## LP-05：经验包签名与来源可信度（v2.0.8）

### 目标

在现有 Hash 和 Scope 隔离之上，让经验包来源、完整性和导入策略更明确。

### 实现范围

- 扩展经验包元数据：OMR 版本、创建工具版本、来源 Scope、Proposal 数量、签名算法。
- 默认使用本地 Ed25519 密钥签名；密钥只存用户明确指定的位置，不自动上传或写入项目。
- `export` 支持 `--sign`；`import` 支持 `--require-signature`、`--trusted-key`。
- 未签名包默认仍可 dry-run，但在实际导入时输出明确 warning；`--require-signature` 下拒绝。
- 签名校验失败、版本不兼容、来源冲突和 Hash 不匹配均 fail closed。

### 验收

- 签名包可验证，内容任意改动都会拒绝。
- 未知字段、绝对路径、敏感内容和过大包拒绝。
- dry-run 零写入；冲突批次零部分写入。
- 私钥权限、路径 symlink 和密钥泄露测试通过。
- 不自动批准导入 Proposal，不自动修改 Overlay。

---

## LP-06：文档、Todo 与差距矩阵清理（v2.0.9）

### 目标

消除历史版本残留，建立单一事实源，让用户能准确知道已完成、可用和阻塞内容。

### 实现范围

- 统一 README 中文/英文、CHANGELOG、Todo、Gap Matrix、Evolution 计划的版本和状态。
- 将历史计划文档标注为 Archived，不删除有审计价值的报告。
- 新增“当前可用能力”矩阵：CLI、Desktop 可用范围、需要官方接口的范围。
- 增加文档链接检查和命令示例 Smoke Test。
- 文档不得把 Reasonix 官方未实现接口写成 OMR 已实现。

### 验收

- 不再出现“v2.0.0 进行中”“T14 待实现”等过期表述。
- 所有命令示例可解析，路径和版本不写死到个人机器。
- 中英文 README 的核心状态一致。
- 文档链接检查通过，旧报告内容保持可追溯。

---

## Reasonix Agent 执行协议

```text
请在 oh-my-reasonix 仓库执行 docs/OMR_POST_V2_LOCAL_ENHANCEMENT_PLAN.zh-CN.md。
按 LP-01 → LP-06 顺序执行，每阶段独立提交。
每阶段先写失败测试，再做最小实现；先运行定向测试，再运行全量门禁。
只使用临时目录和 Fake Provider，不读取正式项目，不使用真实 API Key。
遇到宿主接口依赖，标记 BLOCKED，不猜测、不伪造、不修改 Reasonix。
每阶段完成后报告：提交、文件、测试、风险、未完成项。
不要自行创建或移动旧版本 Tag；Tag 必须由用户确认后执行。
```

## 最终完成标准

- LP-01～LP-06 均有代码/文档证据和回归测试；
- `go test ./...`、`go vet ./...`、`go build ./cmd/omr` 通过；
- 所有 CLI Smoke、Fixture Replay、敏感信息和工作区快照检查通过；
- Reasonix 官方依赖项仍单独标记，不混入 OMR 实现。
