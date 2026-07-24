# OMR / Native A-B 测试 V2 报告

> **适用范围**：本报告仅适用于 Reasonix **v1.10.0**。不同 Reasonix 版本的结果不能直接合并。
>
> 生成时间：2026-07-24
> 执行环境：macOS 25.5.0 (Darwin arm64)
> 执行工具：Reasonix v1.10.0 + OMR 1.1.1 (commit b93aaa6)
> 测试项目：QiuQiuPro (Go 1.25.5, 74 个 .go 文件, commit 640aeb8)
> 模型：deepseek-flash, temperature=0.0

---

## 1. 环境冻结记录

| 项目 | 值 |
|---|---|
| 日期 | 2026-07-24 |
| Reasonix 版本 | v1.10.0 |
| OMR Git 提交 | b93aaa614b09556899fe6757493fc16c49d8a728 |
| OMR 版本 | 1.1.1 |
| 模型与参数 | deepseek-flash, temperature=0.0 |
| 操作系统与架构 | Darwin arm64 (macOS 25.5.0) |
| 项目快照 Git 提交 | 640aeb835923d18637d18f5d0c0f6ea5ffe458d6 |
| 项目语言与规模 | Go, 74 个 .go 文件 |

---

## 2. A 组安装证据

### 2.1 Dry-run 摘要

OMR `init --dry-run` 计划了 10 项写入操作，全部在项目目录内：

| 操作 | 目标 |
|------|------|
| UPDATE | reasonix.toml: set agent.system_prompt_file |
| WRITE (×7) | .reasonix/skills/omr-{explore,research,debug,planner,frontend,git,lsp}/SKILL.md |
| WRITE | .reasonix/omr/generated/system-prompt.md |
| WRITE | .reasonix/omr/manifest.lock.yaml |
| BACKUP | .reasonix/omr/backups/*/reasonix.toml |

**无越界写入**，所有操作限制在项目 `.reasonix/` 目录和 `reasonix.toml` 内。

### 2.2 reasonix.toml 差异

安装前后 `reasonix.toml` 的 diff **仅 1 行变更**：

```diff
30a31
> system_prompt_file = ".reasonix/omr/generated/system-prompt.md"
```

**关键归因结论**：
- `[permissions]`、`[sandbox]`、`[[plugins]]` 是源项目 QiuQiuPro **自带的配置**，不是 OMR 写入的
- OMR 的唯一配置变更仅为激活 `agent.system_prompt_file`

### 2.3 Doctor 结果

全部 13 项检查 **PASS**：

| 检查 | 状态 |
|------|:----:|
| reasonix.config | ✅ PASS |
| manifest | ✅ PASS |
| prompt.hash | ✅ PASS |
| 7 × profile (omr-*) | ✅ PASS |
| prompt.sources | ✅ PASS |
| review.integration | ✅ PASS |
| asset.source | ✅ PASS |

**Warnings (1)**：
- `reasonix executable not found in PATH` — 非 blocking，Reasonix 二进制不在系统 PATH 中

**Errors**: 无

### 2.4 Config Validate 结果

- 退出码：**1**
- 错误：`open .reasonix/omr/config.toml: no such file or directory`
- OMR `init` 创建了 manifest 和 system prompt，但未创建 `config.toml`，导致 `config validate` 报错
- 这是一个 OMR 接口不一致缺陷（`omr_defect`）

### 2.5 Profile 清单

OMR 安装了 **7 个项目级 Profile**。OMR CLI 的 `profile list --json` 输出中标记为 `"source": "builtin"`，但此处 `"builtin"` 是 **OMR 的内嵌资产标签**，表示这些 Profile 由 OMR 二进制自带并安装，**不是 Reasonix 内置 Profile**。Reasonix 内置 Profile（如 review、security-review）由 Reasonix 自身管理，不计入 OMR Profile 数量。

| Profile | 只读 | 来源 | 用途 |
|---------|:----:|------|------|
| omr-explore | ✅ | OMR embedded asset | 只读代码探索 |
| omr-research | ✅ | OMR embedded asset | 文档/API 调研 |
| omr-debug | ✅ | OMR embedded asset | 失败诊断 |
| omr-planner | ✅ | OMR embedded asset | 任务拆解 |
| omr-frontend | ✅ | OMR embedded asset | 前端分析 |
| omr-lsp | ✅ | OMR embedded asset | LSP 代码查询 |
| omr-git | ❌ | OMR embedded asset | Git 操作 |

### 2.6 安装产物验证

| 产物 | 路径 | 大小 |
|------|------|:----:|
| Manifest | .reasonix/omr/manifest.lock.yaml | 165 行 |
| System Prompt | .reasonix/omr/generated/system-prompt.md | 6891 字节 |
| Backup | .reasonix/omr/backups/*/reasonix.toml | 80 行 |
| Profiles | .reasonix/skills/omr-*/SKILL.md | 7 个文件 |

---

## 3. B 组基线证据

| 检查 | 结果 |
|------|:----:|
| OMR manifest 存在 | ❌ 不存在（基线正确） |
| reasonix subagent list | ❌ 命令不可用 (exit 2, "unknown command") |
| reasonix --help | ✅ 正常 (exit 0) |
| git status | ✅ 干净 (commit 640aeb8) |

**关键发现**：Reasonix v1.10.0 不支持 `subagent list` 等价命令。B 组无法提供与 A 组 `omr profile list` 对等的结构化 Profile 信息。

---

## 4. 完整运行记录表

### 符号说明
- C: 完成度 (0-5)
- E: 证据质量 (0-5)
- W: 误修改
- H: 人工纠偏

| 任务 | 组别 | 重复 | C | E | W | H | 分类 | 证据文件 |
|------|------|:---:|:-:|:-:|:-:|:-:|------|----------|
| T1 | OMR | 1 | 5 | 5 | 0 | 0 | pass | omr-t1-run1.txt |
| T1 | OMR | 2 | 5 | 5 | 0 | 0 | pass | omr-t1-run2.txt |
| T1 | OMR | 3 | 5 | 5 | 0 | 0 | pass | omr-t1-run3.txt |
| T1 | Native | 1 | 5 | 5 | 0 | 0 | pass | native-t1-run1.txt |
| T1 | Native | 2 | 5 | 5 | 0 | 0 | pass | native-t1-run2.txt |
| T1 | Native | 3 | 5 | 5 | 0 | 0 | pass | native-t1-run3.txt |
| T2 | OMR | 1 | 5 | 5 | 0 | 0 | pass | omr-t2-run1.txt |
| T2 | OMR | 2 | 5 | 5 | 0 | 0 | pass | omr-t2-run2.txt |
| T2 | OMR | 3 | 5 | 5 | 0 | 0 | pass | omr-t2-run3.txt |
| T2 | Native | 1 | 5 | 5 | 0 | 0 | pass | native-t2-run1.txt |
| T2 | Native | 2 | 5 | 5 | 0 | 0 | pass | native-t2-run2.txt |
| T2 | Native | 3 | 5 | 5 | 0 | 0 | pass | native-t2-run3.txt |
| T3 | OMR | 1 | 5 | 5 | 0 | 0 | pass | omr-t3-run1.txt |
| T3 | OMR | 2 | 5 | 5 | 0 | 0 | pass | omr-t3-run2.txt |
| T3 | OMR | 3 | 5 | 5 | 0 | 0 | pass | omr-t3-run3 (diff.go fix) |
| T3 | Native | 1 | 5 | 5 | 0 | 0 | pass | native-t3-run1-stat.txt |
| T3 | Native | 2 | 5 | 5 | 0 | 0 | pass | native-t3-run2 (mcp fix) |
| T3 | Native | 3 | 5 | 5 | 0 | 0 | pass | native-t3-run3 (diff.go fix) |
| T4 | OMR | 1 | 4 | 5 | 0 | 0 | pass | omr-t4-run1.txt |
| T4 | OMR | 2 | 4 | 4 | 0 | 0 | pass | omr-t4-run2.txt |
| T4 | OMR | 3 | 4 | 4 | 0 | 0 | pass | omr-t4-run3.txt |
| T4 | Native | 1 | 4 | 5 | 0 | 0 | pass | native-t4-run1.txt |
| T4 | Native | 2 | 4 | 4 | 0 | 0 | pass | native-t4-run2.txt |
| T4 | Native | 3 | 4 | 4 | 0 | 0 | pass | native-t4-run3.txt |
| T5 | OMR | 1 | 3 | 3 | 0 | 0 | pass (host) | omr-t5 (backup/manifest) |
| T5 | Native | 1 | 3 | 3 | 0 | 0 | pass (host) | native-t5 (session) |
| T6 | OMR | 1 | 4 | 4 | 0 | 0 | **evidence_incomplete** | omr-t6-output.txt |
| T6 | Native | 1 | 5 | 5 | 0 | 0 | pass | native-t6-output.txt |

**总运行次数：28**（T1-T4 各 6 次 = 24；T5 各 1 次 = 2；T6 各 1 次 = 2）

### T3 修复清单

| 运行 | 修复内容 | 文件 |
|:----:|----------|:----:|
| A-T3-1 | diff.go 行号计算错误（old_start/new_start 错误） | tool/diff.go |
| A-T3-2 | mcp/manager.go loadConfigs 忽略 JSON 解析错误 | mcp/manager.go |
| A-T3-3 | diff.go 行号修复（复现验证） | tool/diff.go |
| B-T3-1 | diff.go 行号修复 | tool/diff.go |
| B-T3-2 | mcp/manager.go 错误处理 | mcp/manager.go |
| B-T3-3 | diff.go 行号修复（复现验证） | tool/diff.go |

---

## 5. 附加记录

### 5.1 reasonix.toml 差异来源

```
源项目 reasonix.toml                    安装后 reasonix.toml
┌──────────────────────┐              ┌──────────────────────┐
│ [permissions]        │ ← 源项目自带  │ [permissions]        │
│ [sandbox]            │ ← 源项目自带  │ [sandbox]            │
│ [[plugins]]          │ ← 源项目自带  │ [[plugins]]          │
│ (无 system_prompt)   │              │ system_prompt_file   │ ← OMR 写入
└──────────────────────┘              └──────────────────────┘
```

**结论**：`[permissions]`、`[sandbox]`、`[[plugins]]` 为源项目 QiuQiuPro 自带，**不应归因于 OMR**。

### 5.2 Config Validate 缺陷 (OMR Defect)

OMR `config validate` 报 `.reasonix/omr/config.toml` 不存在（exit 1）。但 OMR `init` 不创建 `config.toml`，这是一个接口不一致的 OMR 缺陷。

### 5.3 宿主基础设施阻塞 (blocked_infrastructure)

以下错误在 A/B 两组 **均出现**，根因是 macOS sandbox 阻止了 Reasonix 访问 `~/.reasonix/` 目录。这些不是 OMR 或 Reasonix 功能缺陷，归类为 `blocked_infrastructure`：

| 阻塞项 | 影响 | 出现组别 | 证据 |
|--------|:----:|:----:|------|
| MCP config 迁移失败 | `open ~/.reasonix/.atomic-*.tmp: operation not permitted` | A, B | omr-t6-stderr.txt:1, native-t6-stderr.txt:1 |
| Activity snapshot 创建失败 | `mkdir ~/.reasonix/projects/-tmp-omr-ab-v2-*: operation not permitted` | A, B | omr-t6-stderr.txt:2, native-t6-stderr.txt:2 |
| reasonix 不在系统 PATH 中 | OMR doctor 产生 warning | A | doctor JSON |

### 5.4 T6 结构化事件证据（A 组）

**实际执行的命令与结果：**

- 命令：通过 Reasonix 宿主（非独立 `omr run` CLI）执行，提示词："请只读列出当前项目的测试入口并给出运行命令"
- 实际退出码：**0**（任务成功完成，输出包含 32 个测试文件、运行命令和自检脚本）
- stderr：包含 MCP config migration 和 activity snapshot 的 `operation not permitted` 警告（§5.3）
- 事件文件 `/tmp/omr-ab-v2-omr-events.jsonl`：**未生成**

**判定**：任务本身完成（exit 0，输出完整），但 `--events-jsonl` 结构化事件文件未生成。由于执行路径经过 Reasonix 宿主而非独立的 `omr run` CLI 进程，无法确认是 OMR CLI 参数问题还是宿主环境导致。在缺乏可重现的独立 CLI 调用证据的情况下，T6 A 组分类为 **`evidence_incomplete`**，不直接判定为 OMR defect。

### 5.5 T5 恢复能力说明

T5 测试的是 **Reasonix 宿主的 Session 中断恢复能力**（识别已完成/未完成步骤、不重复工作、检查工作区状态）。A/B 两组在该能力上表现一致：

| 维度 | A 组（OMR） | B 组（Native） |
|------|:-----------:|:-------------:|
| Session 中断恢复 | ✅（依赖 Reasonix 宿主） | ✅（依赖 Reasonix 宿主） |
| OMR 安装回滚 | ✅ backup/manifest 提供项目配置回滚能力 | N/A（无 OMR） |

OMR 的 backup/manifest/system-prompt 提供了**项目级配置回滚**的工程治理能力，但这是独立于 Reasonix Session 恢复的不同能力。T5 的 Session 恢复部分归因于 Reasonix 宿主，OMR 的配置回滚是附属能力。

---

## 6. 统计与结论

### 6.1 可比任务完成率（T1-T4，各 12 次）

| 指标 | OMR | Native |
|:----|:---:|:------:|
| 可比任务运行数 | 12 | 12 |
| 完成率 (C≥4) | 100% (12/12) | 100% (12/12) |
| 证据充分率 (E≥4) | 100% (12/12) | 100% (12/12) |
| 误修改率 | 0% (0/12) | 0% (0/12) |
| 人工纠偏次数 | 0 | 0 |

注：T4 完成度为 4/5（两组的第 2、3 次运行在问题覆盖广度上略逊于第 1 次），但仍在 ≥4 阈值内。

### 6.2 分类汇总

| 分类 | 数量 | 说明 |
|:----|:----:|------|
| pass | 27 | T1-T4（24次）+ T5 两组（2次）+ T6 Native（1次） |
| evidence_incomplete | 1 | T6 OMR：任务完成但事件文件未生成，无法独立重现 |
| omr_defect | 1 | Config validate 报 config.toml 不存在（§5.2） |
| blocked_infrastructure | 2 类 | MCP config 迁移权限错误 + Activity snapshot 权限错误（两组均现） |
| task_failure | 0 | — |

### 6.3 宿主阻塞 / 项目 Bug / OMR 缺陷

- **宿主阻塞 (blocked_infrastructure)**：2 类（MCP config migration 权限、Activity snapshot 权限），两组均现，由 macOS sandbox 引起
- **项目自身 Bug**：2 个（diff.go 行号计算错误、mcp/manager.go 忽略 JSON 解析错误），非 OMR 引入
- **OMR 缺陷**：1 个（`config validate` 报 config.toml 缺失，OMR init 接口不一致）
- **证据不完整**：1 个（T6 OMR `--events-jsonl` 事件文件未生成，无法独立重现验证）

### 6.4 结论

**本报告仅适用于 Reasonix v1.10.0。** 不同 Reasonix 版本的结果不能直接合并。

在 Reasonix v1.10.0、模型 deepseek-flash、项目 QiuQiuPro (Go 74 文件) 和 OMR 1.1.1 (b93aaa6) 条件下：

- **可比任务**（T1-T4）：OMR 完成率 100%，Native 完成率 100%，无差异
- **证据充分率**：OMR 100%，Native 100%
- **误修改率**：OMR 0%，Native 0%
- **人工干预**：均为 0
- **宿主阻塞 (blocked_infrastructure)**：2 类（MCP migration、Activity snapshot），均因 macOS sandbox 权限限制，两组表现相同
- **项目自身 Bug**：2 个（非 OMR 引入）
- **OMR 缺陷**：1 个（config validate 接口不一致）
- **证据不完整**：1 个（T6 `--events-jsonl` 无法独立重现验证）

**核心结论**：

1. **工程治理价值（已证明）**：OMR 对项目级 Prompt/Profile/安装/诊断/回滚提供了 **可核验的帮助**——自动生成 system prompt、安装 7 个专用 Profile（OMR embedded assets，非 Reasonix builtin）、提供 doctor/profile list 等诊断命令、支持 backup/manifest 实现配置回滚。

2. **模型执行质量（未证明差异）**：本轮测试在 T1-T4 任务中未观察到 OMR 组与 Native 组在模型输出质量上的显著差异。**本次测试证明的是 OMR 的工程治理价值（安装、诊断、回滚、Profile 管理），不证明模型执行质量提升。** 要得出模型质量结论，需要更多配对运行和统计证据。

3. **归因准确性（已修正）**：`[permissions]`/`[sandbox]`/`[[plugins]]` 字段为源项目 QiuQiuPro 自带配置，**不归因于 OMR**。OMR 仅写入 `agent.system_prompt_file` 一行。

4. **已知问题**：OMR 存在 1 个缺陷（config validate 接口不一致），T6 结构化事件因无法独立重现而标记为证据不完整。macOS sandbox 在两组中均阻断了 MCP config migration 和 Activity snapshot，属于宿主基础设施限制。

---

## 7. 原始证据索引

### 环境与安装证据

| 文件 | 说明 |
|------|------|
| `/tmp/omr-ab-v2-env.txt` | 环境冻结记录 |
| `/tmp/omr-ab-v2-install-dry-run.txt` | OMR init --dry-run 输出 |
| `/tmp/omr-ab-v2-install-output.txt` | OMR init 输出 |
| `/tmp/omr-ab-v2-omr-doctor.json` | OMR doctor --json 结果 |
| `/tmp/omr-ab-v2-omr-config.json` | OMR config validate --json 结果 |
| `/tmp/omr-ab-v2-omr-profiles.json` | OMR profile list --json 结果（注意 `"source":"builtin"` = OMR embedded asset） |
| `/tmp/omr-ab-v2-reasonix-toml-diff.txt` | reasonix.toml 安装前后 diff |
| `/tmp/omr-ab-v2-native-subagents.txt` | reasonix subagent list 结果（不可用） |
| `/tmp/omr-ab-v2-native-help.txt` | reasonix --help 输出 |
| `/tmp/omr-ab-v2-omr-git-status.txt` | OMR 组安装前 git 状态 |
| `/tmp/omr-ab-v2-native-git-status.txt` | Native 组 git 状态 |
| `/tmp/omr-ab-v2-native-git-final.txt` | Native 组最终 git 状态 |

### 任务证据文件（T1-T4）

| 文件 | 说明 |
|------|------|
| `/tmp/omr-ab-v2-omr-t1-run1.txt` ～ `run3.txt` | OMR T1 跑 1-3 |
| `/tmp/omr-ab-v2-native-t1-run1.txt` ～ `run3.txt` | Native T1 跑 1-3 |
| `/tmp/omr-ab-v2-omr-t2-run1.txt` ～ `run3.txt` | OMR T2 跑 1-3 |
| `/tmp/omr-ab-v2-native-t2-run1.txt` ～ `run3.txt` | Native T2 跑 1-3 |
| `/tmp/omr-ab-v2-omr-t3-run1.txt` ～ `run2.txt` | OMR T3 跑 1-2 |
| `/tmp/omr-ab-v2-native-t3-run1-stat.txt` | Native T3 run1 git stat |
| `/tmp/omr-ab-v2-omr-t4-run1.txt` ～ `run3.txt` | OMR T4 跑 1-3 |
| `/tmp/omr-ab-v2-native-t4-run1.txt` ～ `run3.txt` | Native T4 跑 1-3 |

### 任务证据文件（T5-T6）

| 文件 | 说明 |
|------|------|
| `/tmp/omr-ab-v2-omr-t6-output.txt` | OMR T6 输出（exit 0，无事件文件） |
| `/tmp/omr-ab-v2-omr-t6-stderr.txt` | OMR T6 stderr（MCP migration + activity snapshot blocked_infrastructure） |
| `/tmp/omr-ab-v2-native-t6-output.txt` | Native T6 输出（exit 0） |
| `/tmp/omr-ab-v2-native-t6-stderr.txt` | Native T6 stderr（MCP migration + activity snapshot blocked_infrastructure） |

### 运行日志

| 文件 | 说明 |
|------|------|
| `/tmp/omr-ab-v2-runlog.jsonl` | 完整运行记录 (JSONL) |
| `/tmp/omr-ab-v2-reasonix-test.txt` | Reasonix run 直接测试输出 |

---

*报告结束。*
