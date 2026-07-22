# oh-my-reasonix 后续自主开发任务书

> 用途：将本文完整交给 Reasonix，让它在当前仓库中按顺序自主开发、测试和提交。
>
> 当前仓库：`oh-my-reasonix`
>
> 目标：在不需要用户辅助测试的前提下，持续完成 OMR 的自动化能力；只有真实 Reasonix Session、Hook 或后台任务联调时，才请求用户介入。

## 1. 执行规则

1. 先阅读本文、`AGENTS.md`、`README.md` 和 `docs/OMR_VS_OMO_GAP_MATRIX.zh-CN.md`。
2. 每次只完成一个任务，不要把多个任务合并成一个不可审查的大改动。
3. 先写回归测试或 Fixture，再修改实现。
4. 每个任务完成后运行：

   ```bash
   gofmt -w <changed-go-files>
   git diff --check
   go test ./...
   go vet ./...
   ./tests/cli_smoke.sh
   ```

5. 测试必须使用临时目录，不得读取、覆盖或删除真实用户项目。
6. 不修改全局 PATH、API Key、Reasonix 二进制或用户级配置。
7. 保留仓库中已有的未跟踪文件，不要擅自清理：`omr`、`.reasonix/`。
8. 每项任务通过验证后单独提交，提交信息使用中文并说明行为变化。
9. 任务失败时保留失败证据，先修复当前任务，不跳到后续任务。
10. 不复制 Reasonix 原生 Todo、Session、Hook 或后台任务状态机。

## 2. 当前已完成能力（不要重复实现）

- 项目级 init、upgrade、uninstall、dry-run、备份与冲突保护；
- Prompt Composer、Manifest、来源 Hash 和漂移诊断；
- `omr-explore`、`omr-research`、`omr-debug`、`omr-planner`、`omr-frontend`；
- Category 路由、Profile 禁用列表、模型/Prompt/只读配置；
- `doctor`、`config validate`、`config schema`、`profile list`；
- 环境变量展开和 `#`/行尾 `//` 注释；
- `OMR_REASONIX_BIN` 可执行文件配置；
- Session resume/copy/export 转发；
- 离线质量回放、成本门禁、并发、Readiness 指标和 CLI Smoke。

## 3. OMR 仓库内任务

### OMR-01：配置 JSONC 兼容

目标：支持 `.reasonix/omr/config.jsonc`，同时保持现有 TOML 配置兼容。

要求：

- 支持 JSONC 的 `//` 与 `/* ... */` 注释；
- JSONC 与 TOML 映射到同一个内部 Config 类型；
- `config validate`、`doctor`、`upgrade` 使用同一解析入口；
- 解析失败必须返回文件路径、行列信息或明确字段错误；
- 不覆盖原始配置。

验收：新增 JSONC 单测、无效 JSONC 单测和 CLI Smoke；`config validate --json` 输出与 TOML 一致。

### OMR-02：配置格式迁移

目标：增加 `omr config migrate`，把旧 TOML 转换为 JSONC。

要求：

- 默认只输出迁移计划，不写文件；
- `--write` 才执行写入；
- 原文件生成 `.bak` 备份；
- 字段、路由、Profile、环境变量表达式保持等价；
- 重复执行必须幂等；
- 迁移冲突不得静默覆盖。

验收：迁移前后 Config 结构相等，原文件、备份和目标文件均有测试。

### OMR-03：安装与回滚边界

目标：继续扩大安装链路自动化覆盖。

必须覆盖：

- 用户修改生成 Prompt 后升级；
- 用户修改 Profile 后升级；
- Manifest 缺失；
- 安装中断后的恢复；
- 卸载冲突；
- 外部资产目录与嵌入资产不一致；
- dry-run 不产生任何文件。

验收：所有场景均使用临时目录；成功路径和阻断路径都有测试；不得修改真实项目。

### OMR-04：质量 Fixture 扩展

新增或扩展离线 Fixture，覆盖：

- 测试失败 → 修复 → 重测；
- 空响应与重复操作；
- Review 阻断；
- Prompt/Profile 漂移；
- 成本超限；
- 并发上限；
- Readiness block/recovery；
- Session 恢复证据缺失。

验收：

```bash
go run ./cmd/omr benchmark quality \
  --replay \
  --fixtures benchmarks/fixtures \
  --min-qualified-rate 1
```

### OMR-05：质量报告 Schema

目标：固定质量报告的机器可读契约。

必须校验：

- `fixture_count`、`evaluated_count`、`qualified_rate`；
- `metrics`、`cost`、`currency`；
- `readiness_checks`、`readiness_blocks`、`readiness_recoveries`；
- `evaluations` 和 `failures`。

验收：正常报告通过；缺字段、类型错误、负成本和非法比例被拒绝。

### OMR-06：Profile 资产扩展

只有确认 Reasonix 原生工具存在后，按以下顺序逐个实现：

```text
omr-git → omr-ast → omr-browser → omr-visual
```

每个 Profile 必须包含：

- SKILL 文件和明确 frontmatter；
- allowed tools；
- 只读/可写声明；
- 输入输出契约；
- 嵌入资产；
- Manifest Hash；
- Doctor 和安装测试；
- 至少一个离线 Fixture。

若宿主工具不存在，记录调查结果并跳过，不得伪造能力。

### OMR-07：Claude 配置兼容

分任务实现，不要一次性重写：

1. `.claude/rules` → OMR 规则读取；
2. `.claude/skills` → Reasonix Skill 映射；
3. `.claude/agents` → OMR Profile 映射；
4. `.claude/mcp.json` 只读导入；
5. Claude Hooks 转换为策略提示，不复制宿主状态机。

所有导入都必须支持 dry-run、冲突报告和回滚。

## 4. Reasonix 宿主接口任务

以下任务不能只靠 OMR Prompt 完成，应先在 Reasonix 提供稳定机器接口。除非用户明确要求，否则不要把这些实现混入 OMR。

### RX-01：Session 列表

建议接口：

```bash
reasonix session list --json
```

返回 Session ID、项目路径、状态、更新时间、未完成 Todo 数和占用状态。

### RX-02：Session 状态与内容

建议接口：

```bash
reasonix session status <id> --json
reasonix session show <id> --json
```

返回当前 Todo、阶段、最近事件、最近工具调用、错误和恢复父链。

### RX-03：Hook 诊断

建议接口：

```bash
reasonix hook list --json
reasonix hook status --json
```

返回 Hook 来源、启用状态、执行顺序和最近失败原因。

### RX-04：后台任务状态

建议接口：

```bash
reasonix task list --json
reasonix task show <id> --json
```

返回任务 ID、Profile、状态、时间、结果引用和错误。

### RX-05：恢复链

建议接口：

```bash
reasonix session recovery <id> --json
```

返回失败原因、恢复次数、恢复节点、当前恢复点和是否需要人工介入。

### RX-06：结构化事件流

建议支持：

```bash
reasonix run ... --events-jsonl <path>
```

至少包含：`session_started`、`todo_write`、`tool_call`、`tool_result`、`hook_block`、`review_report`、`task_started`、`task_finished`、`session_paused`、`session_resumed`、`session_completed`。

## 5. OMR 与 Reasonix 联调任务

只有 RX 接口稳定后才执行：

### INT-01：`omr session list`

只读转发 Reasonix Session JSON，不维护第二套状态。

### INT-02：`omr session status`

展示当前 Todo、阶段、最近错误和恢复点。

### INT-03：`omr hook doctor`

Doctor 检查 OMR 所需 Hook 是否启用。

### INT-04：`omr task list`

展示后台任务状态和结果引用。

### INT-05：后台结果汇聚

只消费 Reasonix 结构化结果，不自行复制后台任务状态机。

### INT-06：真实客户端验证

该任务才需要用户介入：

1. 用户启动 Reasonix；
2. 在指定项目运行任务；
3. 验证 Session 恢复、Hook 阻断和后台任务；
4. 观察 OMR 输出与 Reasonix 状态是否一致。

## 6. 推荐顺序

```text
OMR-01
OMR-02
OMR-03
OMR-04
OMR-05
OMR-06
OMR-07
RX-01
RX-02
RX-03
RX-04
RX-05
RX-06
INT-01
INT-02
INT-03
INT-04
INT-05
INT-06
```

其中 `OMR-06`、`OMR-07` 必须先确认宿主能力；`RX-*` 不应伪装成 OMR 内部功能；`INT-06` 是唯一明确需要用户辅助测试的阶段。

## 7. 最终交付要求

每项任务完成后报告：

- 修改文件；
- 行为变化；
- 测试命令和结果；
- 是否有未解决风险；
- Git commit；
- 是否需要进入下一项。

没有通过自动化验证时，不得宣称任务完成。
