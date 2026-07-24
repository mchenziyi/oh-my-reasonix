# OMR / Native A-B 测试 V2 执行计划

> 用途：交给 Reasonix Agent 执行一次可复核的 OMR 与原生 Reasonix 对照测试。
> 
> 本轮重点不是追求更高分，而是修正 V1 的归因问题，区分 OMR 能力、Reasonix 宿主能力、项目自身问题和测试环境阻塞。

## 1. 固定假设与禁止事项

1. A 组和 B 组必须使用同一台机器、同一 Reasonix 版本、同一模型、同一项目初始快照和同一提示词。
2. A 组只额外安装 OMR；B 组不得执行 OMR 命令，也不得复制 OMR Prompt、Profile、Manifest 或配置。
3. 不把模型随机性、网络超时、宿主沙箱错误或项目自身 Bug 归因于 OMR。
4. 不把 OMR 的 Profile 数量写成“8 个 OMR Profile”。当前 OMR 项目资产为 7 个：6 个只读 Profile（explore、research、debug、planner、frontend、lsp）和 1 个允许 Git 操作的 `omr-git`；Reasonix 内置 Profile 单独统计。
5. 不把 `sandbox`、`permissions` 等字段归因于 OMR，除非安装前后 `reasonix.toml` 差异和 OMR dry-run 明确证明它们由 OMR 写入。OMR 预期只写入 `agent.system_prompt_file`，并安装项目级 OMR 资产。
6. T6 结构化事件如果出现 `operation not permitted`、端口、权限或客户端占用错误，标记为 `blocked_infrastructure`，不得直接计入 OMR 任务失败。
7. 不把单次成本或耗时差异写成产品结论。至少三次同条件重复后，才报告中位数；宿主阻塞时不比较成本。
8. 测试项目内容发送给模型服务前，必须确认用户授权。测试结束后保留证据，但不要提交项目源码、Token、API Key 或完整敏感输出。

## 2. 冻结环境

在执行前记录以下信息。Reasonix Agent 不得在测试中途升级版本、切换模型或修改 OMR 代码。

```text
日期：
Reasonix 版本：
OMR Git 提交：
OMR 版本：
模型与参数：
操作系统与架构：
项目快照 Git 提交：
项目语言与规模：
```

从 OMR 仓库构建临时二进制，避免在目标项目中使用不稳定的跨目录 `go run`：

```bash
OMR_REPO=/path/to/oh-my-reasonix
OMR_BIN=/tmp/omr-ab-v2
cd "$OMR_REPO"
git rev-parse HEAD
go build -o "$OMR_BIN" ./cmd/omr
"$OMR_BIN" version
reasonix --version
```

如果当前 Reasonix 版本不是此前的 v1.10.0，必须在报告中明确写出新版本；不同版本的结果不能直接合并。

## 3. 隔离 A/B 项目

`SOURCE_PROJECT` 必须是没有 OMR 安装产物的只读基线快照。不要用 A 组运行后的目录创建 B 组。

```bash
SOURCE_PROJECT=/path/to/clean/QiuQiuPro
BASE=/tmp/omr-ab-v2
cp -R "$SOURCE_PROJECT" "$BASE-omr"
cp -R "$SOURCE_PROJECT" "$BASE-native"
```

在两个目录分别保存：

```bash
git status --short
git rev-parse HEAD
```

测试结束后不要删除用户已有目录；只清理本轮明确创建的 `/tmp/omr-ab-v2-*` 目录。

## 4. A 组安装证据

```bash
cd /tmp/omr-ab-v2-omr
"$OMR_BIN" init --project-dir . --dry-run > /tmp/omr-ab-v2-install-dry-run.txt
"$OMR_BIN" init --project-dir .
"$OMR_BIN" doctor --project-dir . --json > /tmp/omr-ab-v2-omr-doctor.json
"$OMR_BIN" config validate --project-dir . --json > /tmp/omr-ab-v2-omr-config.json
"$OMR_BIN" profile list --project-dir . --json > /tmp/omr-ab-v2-omr-profiles.json
git diff -- reasonix.toml
```

验收：

- dry-run 只计划项目目录内写入；
- `reasonix.toml` 的 OMR 变更只有明确记录的 `agent.system_prompt_file`，其他字段必须标为项目/宿主原有变更；
- Doctor 没有 blocking error；MCP 不可用只能是 warning；
- Manifest、生成 Prompt、Profile Hash 和备份存在；
- Profile 数量按 OMR 项目 Profile 与 Reasonix builtin 分开记录。

## 5. B 组 Native 基线证据

```bash
cd /tmp/omr-ab-v2-native
test ! -e .reasonix/omr/manifest.lock.yaml
reasonix subagent list > /tmp/omr-ab-v2-native-subagents.txt
reasonix --help > /tmp/omr-ab-v2-native-help.txt
git status --short
```

B 组不运行 `omr doctor`、`omr profile list` 或 `omr run`。需要比较结构化事件时，使用 Reasonix 当前版本实际支持的原生命令，并记录“不支持”而不是猜测。

## 6. 可比任务矩阵

每个任务先在 A 组执行，再从同一初始快照重置/复制到 B 组执行。T1～T4 建议各运行 3 次；T5、T6 因需要人工中断或宿主状态，各运行 1 次并完整保存原始证据。

### T1：探索与证据

```text
请只读分析当前项目：列出主要模块、程序入口、测试入口、关键依赖和潜在高风险区域。不要修改文件。每个结论引用实际文件路径，并区分已确认事实与推测。
```

### T2：计划与边界

```text
请把“为当前项目增加一个可配置的健康检查命令”拆成最小实施任务。先列假设、影响文件、风险、验收条件和测试命令，暂时不要修改代码。
```

### T3：真实最小修复

```text
请先定位当前项目中一个有测试或构建证据的最小 Bug，给出根因和影响文件。确认后只做最小修复，运行相关测试并报告修改文件、测试命令和结果。不要顺手重构无关代码。
```

T3 是唯一允许修改文件的任务。A、B 两组必须使用独立目录，并在结束时记录 `git diff --stat`、测试结果和是否越界修改。

### T4：安全 Review

```text
请对当前项目做一次只读安全 Review，重点检查输入校验、路径越界、敏感信息泄露、错误处理和测试缺口。每个问题给出严重级别、文件路径、证据和修复建议；没有证据不要猜测。
```

### T5：恢复

先让会话在中途停止，再发送：

```text
请从上次中断的位置继续。先说明已完成和未完成的步骤，不要重复已完成的工作；继续前先检查工作区状态。
```

记录是否重复执行、是否保留上下文、是否需要人工干预。若两组都表现相同，结论应归为宿主能力，不算 OMR 独有收益。

### T6：结构化事件与 OMR CLI

A 组执行：

```bash
cd /tmp/omr-ab-v2-omr
"$OMR_BIN" run --project-dir . --events-jsonl /tmp/omr-ab-v2-omr-events.jsonl --json "请只读列出当前项目的测试入口并给出运行命令"
```

B 组执行 Reasonix 原生等价任务，并保存宿主支持的结构化输出。若 A 组失败，必须保存完整 stderr、退出码、命令参数（脱敏）和 `git status`，然后单独做一次诊断复现；不能用后续切换到 Reasonix CLI 的成功结果覆盖 A 组失败。

T6 判定：

- 成功且事件完整：`pass`；
- OMR CLI 参数/事件校验错误：`omr_defect`；
- 端口、权限、沙箱、客户端占用或宿主接口缺失：`blocked_infrastructure`；
- 项目任务本身失败：`task_failure`。

## 7. 记录格式

每次运行至少记录：`run_id`、组别、任务、重复编号、Reasonix/OMR 版本、模型、开始/结束时间、退出码、完成度 0～5、证据质量 0～5、是否误改、人工纠偏次数、Token/成本、失败分类和证据文件路径。

| 任务 | 组别 | 重复 | 完成度 | 证据 | 误改 | 人工纠偏 | 耗时 | 成本 | 分类 | 证据文件 |
|---|---|---:|---:|---:|---|---:|---:|---:|---|---|
| T1 | OMR/Native | 1-3 | | | | | | | | |
| T2 | OMR/Native | 1-3 | | | | | | | | |
| T3 | OMR/Native | 1-3 | | | | | | | | |
| T4 | OMR/Native | 1-3 | | | | | | | | |
| T5 | OMR/Native | 1 | | | | | | | | |
| T6 | OMR/Native | 1 | | | | | | | | |

附加记录单独列出：

- OMR Doctor、Config Validate 和 Profile 清单；
- A/B 两组安装前后 `reasonix.toml` 差异；
- OMR Profile 与 Reasonix builtin Profile 数量；
- MCP、sandbox、permissions 等字段的来源证据；
- 宿主阻塞、项目 Bug 和 OMR 缺陷的分类证据。

## 8. 统计与结论门槛

只比较条件完全一致且分类为 `pass` 的运行。对 T1～T4 计算每组中位数和完成率；T5、T6 单独报告，不用单次结果推断质量优势。

报告至少包含：

- 可比任务完成率和证据充分率；
- 误修改率和人工干预次数；
- 恢复成功率；
- 同条件下的中位耗时和成本；
- `blocked_infrastructure`、`task_failure`、`omr_defect` 各自数量；
- OMR 提供的项目治理能力与 Native 原生能力边界。

禁止使用以下表述，除非有足够重复、配对结果和统计证据：

- “OMR 已证明优于 Native”；
- “OMR 成本更低/更高”；
- “sandbox/permissions 是 OMR 实现但未生效”。

推荐结论模板：

```text
在 Reasonix <版本>、模型 <模型>、项目 <项目> 和 OMR <提交> 条件下：
- 可比任务：OMR <...>，Native <...>；
- 证据充分率：OMR <...>，Native <...>；
- 误修改率：OMR <...>，Native <...>；
- 人工干预中位数：OMR <...>，Native <...>；
- 宿主阻塞：<...>；项目自身 Bug：<...>；OMR 缺陷：<...>。

结论：OMR 对项目级 Prompt/Profile/安装/诊断/回滚提供了 <有/无> 可核验帮助；
在模型执行质量、宿主 Session/恢复和成本方面，当前 <有/无> 足够配对证据下结论。
```

## 9. 最终交付物

Reasonix Agent 完成后只提交：

1. `docs/OMR_AB_TEST_V2_REPORT.zh-CN.md`：完整记录、原始证据索引和谨慎结论；
2. 必要时的脱敏 JSON 元数据；
3. 不修改 OMR 代码、不提交项目源码、Token、API Key、完整模型输出或 `/tmp` 临时目录。

如果 T6 仍为 `blocked_infrastructure`，先提交报告并标记 INT-06/宿主联调阻塞，不要为了得到 6/6 而更换执行路径重算结果。
