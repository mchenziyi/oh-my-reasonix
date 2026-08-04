# OMR v2.0.0 自动自进化开发计划

> 状态：EV-MVP-00～10 已完成；v2.0.0 MVP 已交付，观察期自动回滚增强已实现
> 目标版本：OMR v2.0.0
> 核心定位：OMR 是 Reasonix 的项目级外置策略大脑；Reasonix 保持 Agent 与执行引擎职责。

## 1. 产品目标

Reasonix 装载 OMR 后，继续负责理解任务、模型推理、工具调用、Subagent、Session、Task、权限和实际代码修改。OMR 不复制 Agent Runtime，只在现有安装、Profile、规则、Hook、质量基准和诊断能力之上增加自动自进化闭环。

自进化的对象是 OMR 的项目级 Prompt、Profile、编排规则、Review 规则、路由和质量 Fixture。最终效果是：OMR 持续改进自身，使装载 OMR 的 Reasonix 更适合当前项目；Reasonix 二进制和宿主核心保持不变。

## 2. 非目标

- 不实现第二套 Agent、Session、Task、Todo、权限或后台任务状态机；
- 不自动修改 Reasonix 二进制或官方源码；
- 不自动修改 OMR Go 源码、API Key、全局配置、安全 Hook 或用户手写 Prompt；
- 不读取 Reasonix 私有 Session 文件，不以文件系统猜测宿主状态；
- 不把单次成功、单次失败或模型随机波动当成可晋级经验；
- 不以“自动触发”为由绕过验证、审批、审计或回滚。

## 3. 职责边界

| 能力 | Reasonix | OMR |
|---|:---:|:---:|
| 任务理解、推理与工具执行 | ✅ | — |
| Session、Task、Subagent、权限 | ✅ | — |
| 产生公开事件和执行证据 | ✅ | 消费 |
| 任务质量记录与长期聚合 | 提供数据 | ✅ |
| 失败/成功模式识别 | 执行语义分析 | 组织与约束 |
| 生成改进提案 | 执行推理 | 定义 Schema 与边界 |
| Control/Candidate 回放 | 执行任务 | 组织、评分和留证 |
| OMR 策略生效、版本与回滚 | — | ✅ |

## 4. 自动闭环

```text
Reasonix 完成日常任务
        ↓ 自动采集
OMR Episode Store
        ↓ 达到阈值
Trigger Engine
        ↓
Pattern Analyzer 调用 Reasonix 分析
        ↓
生成受约束 Proposal
        ↓
Control / Candidate 配对回放
        ↓
Promotion Gate
        ↓ 批准后
Evolution Overlay 原子生效
        ↓
观察后续任务，异常自动回滚
```

CLI 只用于状态查看、审批、拒绝、诊断和回滚。正常情况下，用户不需要手动执行分析或生成提案。

## 5. 自动采集入口

### 5.1 OMR 转发运行

通过 `omr run` 执行的任务，在公开 `run_done` 事件后自动生成 Episode。这条路径证据最完整，应作为首个实现和测试入口。

### 5.2 Reasonix 生命周期 Hook

Reasonix Desktop 或原生 CLI 直接执行的任务，在宿主提供稳定生命周期 Hook 时自动通知 OMR。安装前必须探测能力，只注册宿主实际支持的公开 Hook，不猜测事件名称和 payload。

### 5.3 延迟补偿扫描

当宿主没有可靠的任务结束 Hook，OMR 在后续安全入口中读取 Reasonix 公开 Session/Event 接口，补录尚未归档的任务。补偿扫描必须幂等，不能把未知状态伪造成成功。

## 6. Episode 数据模型

每次任务形成一条脱敏的结构化 Episode，至少包含：

```yaml
schema_version: 1
episode_id: ep_...
session_id: session_...
project_scope: project
result: success
started_at: 2026-08-04T00:00:00Z
finished_at: 2026-08-04T00:01:00Z
review:
  requested_changes: 1
tests:
  passed: 12
  failed: 0
corrections:
  user_count: 0
usage:
  input_tokens: 10000
  output_tokens: 1200
evidence_refs:
  - events.jsonl
```

默认不保存完整模型思考、凭据、完整命令正文或不必要的绝对路径。Episode 必须标记证据来源、完整性和宿主版本。

## 7. Trigger Engine

第一版使用确定性阈值，不让模型自行决定是否进化：

| 条件 | 默认阈值 | 动作 |
|---|---:|---|
| 同类失败重复出现 | 3 次 | 生成失败 Pattern |
| 相同 Review 问题重复出现 | 3 次 | 生成 Review 规则候选 |
| 用户纠正同类行为 | 2 次 | 生成工作流候选 |
| 成本持续超过基线 | 连续 3 次 | 生成路由/流程候选 |
| 同一流程稳定成功 | 5 次 | 生成成功 Pattern |
| 累计完成任务 | 10 次 | 执行周期分析 |
| 新策略指标下降 | 2 次 | 自动回滚 |

同一证据不能重复计数；基础设施失败、模型失败、任务失败和判定失败必须分开。

## 8. Proposal 协议

Trigger 命中后，OMR 将脱敏 Pattern、当前策略和允许修改范围交给 Reasonix。Reasonix 只能返回结构化 Proposal，不得直接写入生效目录：

```yaml
schema_version: 1
proposal_id: prop_...
pattern_id: repeated_missing_go_vet
target:
  type: delivery_rule
  path: active/orchestration.md
change:
  operation: append_rule
  content: Go 代码变更后执行 go test ./... 和 go vet ./...
risk: low
expected_effect:
  metric: review_rejection_rate
  direction: decrease
evidence_count: 3
```

所有 Proposal 必须包含来源证据、预期改善指标、风险等级、允许路径和回滚条件。Schema 外字段拒绝解析。

## 9. Evolution Overlay

进化资产不直接覆盖 OMR 内置资产，而是写入项目级覆盖层：

```text
.reasonix/omr/evolution/
├── episodes/
├── patterns/
├── proposals/
├── experiments/
├── snapshots/
├── active/
│   ├── orchestration.md
│   ├── review-rules.md
│   ├── routing.yaml
│   └── profiles/
└── evolution.lock.yaml
```

Prompt 组合顺序固定为：Reasonix Base → 用户 Prompt → OMR 稳定内置 Prompt → 项目 Evolution Overlay。`omr upgrade` 不得覆盖 Overlay，`doctor` 必须校验其 Manifest、Hash、来源和当前状态。

## 10. 允许与禁止的进化对象

允许进入第一阶段：

- 编排和任务拆解规则；
- Profile Prompt 与只读路由；
- Review 检查项和交付门禁；
- 失败恢复策略；
- 项目术语和已确认约定；
- 离线质量 Fixture。

禁止自动修改：

- Reasonix 二进制和源码；
- OMR 可执行代码；
- API Key、权限、Sandbox 和全局配置；
- 安全 Hook；
- 用户原始 Prompt；
- Reasonix 私有状态。

## 11. 自动验证与晋级门禁

每个 Proposal 必须建立隔离的 Control 和 Candidate，使用相同模型、任务、项目快照、运行次数和验收脚本进行配对回放。

| 指标 | 第一版门禁 |
|---|---|
| 原问题修复率 | 必须提高 |
| 自动测试通过率 | 不得下降 |
| Review 阻断率 | 不得恶化 |
| 误修改率 | 不得提高 |
| 用户纠偏次数 | 不得提高 |
| Token 成本 | 默认不得增加超过 15% |
| 执行耗时 | 默认不得增加超过 20% |
| 安全检查 | 必须全部通过 |

单次运行不能晋级。基础设施失败必须排除在模型质量结论之外，且不能被静默丢弃。

## 12. 自动化等级

| 等级 | 行为 |
|---|---|
| L1 观察 | 自动采集、归因和报告，不生成生效变更 |
| L2 建议 | 自动生成、回放和验证，人工批准后生效 |
| L3 受控自治 | 低风险候选通过门禁后自动生效，异常自动回滚 |

v2.0.0 首次交付默认使用 L2。L3 必须在积累足够真实数据后单独评审，不作为 MVP 验收条件。

## 13. 生效、观察与回滚

批准后先创建 Snapshot，再通过原子写入切换 `active/` 和 `evolution.lock.yaml`。新策略进入观察期，默认观察后续 5 个相关任务。

测试通过率下降、Review 问题增加、成本超限、用户明确纠正、Manifest/Hash 异常或原问题再次出现时自动回滚。当前实现先采用确定性保护：批准后观察后续 Episode，累计两次失败即恢复 Snapshot、重建 Prompt/Manifest，并将 Proposal 标记为 `rolled_back`；更细粒度指标将在后续版本增加。

## 14. 控制面 CLI

```bash
omr evolve status
omr evolve proposals
omr evolve report
omr evolve approve <proposal-id>
omr evolve reject <proposal-id>
omr evolve history
omr evolve rollback <version>
omr evolve doctor
```

这些命令不是日常进化触发器。采集、阈值判断、提案和回放由自动闭环触发。

## 15. 开发阶段

| 阶段 | 工作项 | 验收结果 |
|---|---|---|
| EV00 | 协议、边界、隐私和威胁模型 | Schema 与禁止项冻结 |
| EV01 | Episode Store | 自动记录、脱敏、幂等 |
| EV02 | Observer 与补偿扫描 | 三类采集入口可诊断 |
| EV03 | Trigger Engine | 确定性阈值与去重 |
| EV04 | Pattern Analyzer | 重复失败/成功模式聚合 |
| EV05 | Proposal 协议 | Reasonix 生成受限提案 |
| EV06 | Evolution Overlay | 项目级组合、Hash 和冲突检测 |
| EV07 | Replay/A-B Evaluator | Control/Candidate 配对回放 |
| EV08 | Promotion Gate | 审批、原子生效和观察期 |
| EV09 | Snapshot/Rollback | 自动和人工回滚 |
| EV10 | 产品化与长期实验 | 真实项目效果报告 |

## 16. MVP 闭环

v2.0.0 首个可交付闭环只处理一种问题：重复 Review 或测试失败。

```text
重复失败
→ 自动形成 Pattern
→ 自动生成规则 Proposal
→ 自动离线配对验证
→ 用户批准
→ Evolution Overlay 生效
→ 观察并可回滚
```

MVP 必须满足：无需用户手动触发分析；未批准前零生效修改；所有证据可审计；写入原子化；可回滚；不得读取宿主私有状态；不得声称未经对照实验支持的质量提升。

## 17. 与 Reasonix 官方能力的关系

PR #6998 的 Task Monitor 可以提升可观测性，但不是 v2.0.0 MVP 的硬前置。首个闭环可基于现有 `omr run --events-jsonl`、公开 Session/Event 接口、Hook 和离线 Benchmark 开发。

当宿主缺少稳定生命周期事件时，Observer 必须降级为延迟补偿并在 `doctor` 中明确报告，不能通过读取私有文件或返回空数据伪造自动采集成功。
