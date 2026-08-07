# OMR Harness 自进化完整优化方案

> 版本：2026-08-06
> 基线：OMR v2.0.9
> 状态：设计冻结，尚未实施
> 定位：在不修改 Reasonix 模型权重和宿主 Runtime 的前提下，将 OMR 从“安全的 Prompt Overlay 自进化”升级为“证据驱动、可证伪、可回滚的 Harness 自进化系统”。

## 1. 研究依据

本方案以 Lilian Weng 的《Harness Engineering for Self-Improvement》为总览，并吸收其引用的以下研究方向：

- Harness Engineering：工作流、上下文、工具、持久状态、评估和权限共同构成 Agent Harness；
- ACE：将 Context 维护为可增量更新、可去重、可淘汰的结构化 Playbook，而不是反复重写 Prompt；
- MCE：区分“优化 Context 内容”和“优化 Context 管理机制”两个层级；
- Self-Harness：Weakness Mining → Harness Proposal → Proposal Validation；
- AHE：组件可观测性、经验可观测性和决策可观测性；
- Harness Updating / Harness Benefit：能生成 Harness 修改不等于任务 Agent 能从修改中获益；
- Evolutionary Search：保留候选血缘、多样性、预算和停止条件；
- 安全原则：Evaluator、权限、预算、Held-out 数据必须位于进化循环之外。

参考资料：

- <https://lilianweng.github.io/posts/2026-07-04-harness/>
- <https://arxiv.org/abs/2510.04618>
- <https://arxiv.org/abs/2601.21557>
- <https://arxiv.org/abs/2606.09498>
- <https://arxiv.org/abs/2604.25850>
- <https://arxiv.org/abs/2605.30621>

## 2. 当前基线与核心判断

OMR v2.0.9 已具备：

- Episode、Pattern、Proposal、Experiment、Observation；
- 三次同类失败触发提案；
- Promotion Gate、安全验证、人工批准；
- Overlay、Prompt/Manifest 重建、Snapshot、Rollback；
- 生效后观察、连续失败自动回滚；
- 经验包签名、导入导出、修复、清理和报告。

当前闭环已经能够安全地修改 OMR，但仍有三项根本不足：

1. **弱点识别较浅**：主要按 `task_class + failure_class` 聚类，不能证明真实根因相同；
2. **候选验证较弱**：主要证明格式、安全和协议完整，不能证明候选比当前 Harness 更好；
3. **进化对象较窄**：当前主要修改单段 Prompt Overlay，尚未形成结构化 Harness 组件和策略生命周期。

因此下一阶段的第一目标不是扩大自动修改权限，而是先把每次修改变成一条有证据、可证伪、可复现的工程决策。

## 3. 设计原则

### 3.1 信任边界

进化循环永远不能修改：

- Reasonix 二进制、源码和私有状态；
- OMR 验证器、权限策略、安全 Hook 和审批规则；
- Held-out Fixture、评分权重和预算上限；
- API Key、模型选择、推理预算和全局配置；
- 用户手写 Prompt、规则和未授权文件；
- 本方案定义的 Trust Root。

### 3.2 默认自治等级

默认保持 L2：

```text
自动采集
→ 自动诊断
→ 自动生成多个候选
→ 自动离线验证
→ 人工批准
→ 自动观察
→ 异常自动回滚
```

不得默认启用自动批准。只有在长期配对数据充分、风险等级低、用户单独授权后，才可评估 L3。

### 3.3 最小可归因修改

- 一个 Proposal 默认只修改一个 Harness Component；
- 一个 Candidate 默认只包含一组相关 Strategy Item；
- 不同时改变模型、预算、工具权限和策略内容；
- 无法归因的多组件修改必须拆分或拒绝。

### 3.4 失败优先

- 保存 rejected、rolled_back、superseded 和 inconclusive 结果；
- 相同失败候选不得被反复生成；
- 无法证明改善时输出 `inconclusive`，不得输出“提升”；
- 负结果是搜索空间剪枝证据，不是可以清理掉的噪音。

## 4. 目标架构

```text
Reasonix 公开事件 / omr run
              ↓
       Episode Collector
              ↓
    Evidence Normalizer + Redactor
              ↓
      Causal Weakness Miner
              ↓
       Failure Pattern Graph
              ↓
  Bounded Multi-Candidate Proposer
              ↓
       Candidate Sandbox
       ↙               ↘
 Held-in 回放        Held-out 回放
       ↘               ↙
  Multi-Verifier Promotion Gate
              ↓
       Human Decision Point
              ↓
 Structured Playbook / Component Overlay
              ↓
   Online Observation + Drift Detection
              ↓
 Keep / Rollback / Supersede / Retire
```

整个系统分为四个平面：

| 平面 | 职责 |
|---|---|
| Execution Plane | Reasonix 执行真实任务，OMR 不复制 Runtime |
| Evidence Plane | 采集、脱敏、归一化和分层存储执行证据 |
| Evolution Plane | 根因分析、候选生成、回放、比较与决策 |
| Trust Plane | 权限、Verifier、Held-out、预算、签名、审计和回滚 |

## 5. 三类可观测性

### 5.1 Component Observability

建立 Harness Component Registry：

| 组件 | v3 初始权限 | 说明 |
|---|---|---|
| `prompt.strategy` | 可进化 | 结构化策略条目 |
| `profile.instructions` | 建议模式 | Profile 附加规则，不覆盖 Skill 主体 |
| `routing.policy` | 建议模式 | Profile 选择和触发条件 |
| `review.policy` | 可进化 | Review 证据和停止条件 |
| `workflow.policy` | 可进化 | Plan/Test/Review 顺序与重试规则 |
| `memory.playbook` | 可进化 | 长期经验条目 |
| `quality.fixture` | 只建议 | 必须人工确认后才能加入验证集 |
| `tool.description` | 禁止 | 由 Reasonix 官方维护 |
| `tool.implementation` | 禁止 | 不在 OMR 自动进化范围 |
| `middleware` | 禁止 | 后续研究阶段再评估 |
| `omr.source` | 禁止 | 不允许自动修改 OMR Go 源码 |

每个组件必须有：Schema、允许路径、最大大小、渲染器、验证器、回滚策略和所有权声明。

### 5.2 Experience Observability

证据采用三层结构，避免把完整轨迹塞进模型上下文：

```text
L0 Raw Evidence Reference
  仅保存脱敏事件引用、Hash、时间和来源

L1 Episode Analysis
  单任务事实、Verifier 结果、行为摘要、因果假设

L2 Pattern Overview
  多 Episode 聚类、共同机制、反例、置信度
```

默认提案上下文只包含 L1/L2；只有诊断需要时才按引用读取 L0。

### 5.3 Decision Observability

每次修改必须形成 Manifesto：

```yaml
evidence_refs: [episode_x, verifier_y]
root_cause: "任务在修改后未执行最小回归测试"
target_component: workflow.policy
candidate_change: "失败修复后必须执行相关测试"
predicted_fix:
  metric: focused_test_completion_rate
  expected_delta: ">= 0.10"
at_risk_regressions:
  - latency
  - token_cost
acceptance:
  held_in: target_failure_reduced
  held_out: no_blocking_regression
```

后续结果必须回填这条预测是 confirmed、rejected 还是 inconclusive。

## 6. 数据模型 v2

### 6.1 EpisodeV2

在现有 Episode 上增加：

- `model_ref`、`reasonix_version`、`omr_version`；
- `profile_ids`、`harness_revision`；
- `verifier_outcomes`；
- `terminal_cause`；
- `agent_behavior_cause`；
- `suspected_component`；
- `evidence_refs`；
- `duration_ms`、准确的 Token 分项；
- `environment_fingerprint`；
- `completeness` 和 `redaction_status`。

禁止保存完整 Prompt、思考、命令正文、工具参数和凭据。

### 6.2 CausalPattern

不能只按错误字符串聚类，至少包含：

- 表层失败类型；
- Verifier 层终因；
- Agent 行为机制；
- 目标 Harness Component；
- 支持证据和反例；
- 根因置信度；
- 可处理性：`actionable / external / task_specific / unknown`；
- 已尝试且失败的 Candidate ID。

只有 `actionable` 且置信度达到阈值的 Pattern 才能产生 Proposal。

### 6.3 StrategyItem

用结构化 Playbook 替代单段 Overlay：

```yaml
id: strategy_focused_test_after_fix
component: workflow.policy
scope:
  task_class: debug
  model_family: "*"
text: "修复失败后运行最小相关回归测试，再运行扩大范围测试。"
source_pattern: pattern_...
status: active
priority: 50
created_at: ...
expires_at: null
content_sha256: ...
```

支持 `active / shadow / retired / superseded / rolled_back`。

### 6.4 CandidateProposal

新增：

- `target_component`；
- `strategy_items`；
- `parent_candidate_ids`；
- `generation_method`；
- `novelty_fingerprint`；
- `predicted_fix`；
- `at_risk_regressions`；
- `evaluation_plan`；
- `budget_request`；
- `model_scope`；
- `lineage`。

### 6.5 EvaluationRun

分别记录：

- Control Harness Hash；
- Candidate Harness Hash；
- held-in / held-out 数据集 Hash；
- 模型、温度、预算和环境指纹；
- 每个任务的可验证结果；
- 成功率、成本、耗时、误改和人工纠偏；
- 回归列表；
- 判定和不确定性。

## 7. Weakness Mining

### 7.1 两阶段分析

1. **确定性归一化**：从退出码、测试、Review、Hook、文件 Diff 和结构化事件生成事实；
2. **Reasonix 因果分析**：只能基于事实引用输出严格 JSON，不得修改文件。

### 7.2 聚类键

聚类不再固定为 `task_class + failure_class`，而是：

```text
task_class
+ verifier_cause
+ agent_behavior_cause
+ suspected_component
+ model_family
```

模型字段可以配置为泛化或隔离，但报告必须明确范围。

### 7.3 反例要求

Pattern 至少包含：

- 3 条支持 Episode；
- 1 条相似但成功的反例，或显式标记无反例；
- 至少一个外部原因排除项；
- 不得将权限、网络、API Key、宿主崩溃归因给 OMR 策略。

## 8. 多候选生成与搜索

每个合格 Pattern 默认生成 3 个候选：

1. 最小指令修改；
2. 工作流或路由修改；
3. 保守控制候选，例如仅增加验证步骤。

候选必须：

- 修改同一个目标组件；
- 语义互异；
- 不得只是同义改写；
- 与历史 rejected/rolled_back 候选做确定性去重；
- 超出预算时停止生成。

MVP 不引入向量数据库。先用规范化文本 Hash、Token 集合相似度和组件级 Diff 去重。

## 9. Candidate Sandbox 与配对评估

### 9.1 数据集划分

- Held-in：触发该 Pattern 的任务及其最小变体，验证是否修复目标问题；
- Held-out：相邻但未参与提案生成的任务，验证未知回归；
- Canary：安全、权限、路径和凭据测试；
- Long-term：兼容性、维护性和成本测试。

数据集定义、评分器和 Hash 位于 Trust Plane，进化 Agent 只读。

### 9.2 配对原则

Control 与 Candidate 必须固定：

- 同一 Reasonix 版本；
- 同一模型与模型参数；
- 同一任务输入和项目快照；
- 同一工具、权限和预算；
- 相同重复次数；
- 独立运行目录；
- 先后顺序随机或交错，减少时间偏差。

### 9.3 判定

候选只有同时满足以下条件才可进入批准队列：

- Held-in 目标失败显著减少或达到明确验收阈值；
- Held-out 无 blocking regression；
- Canary 全部通过；
- 成本和耗时不超过预算；
- 没有误改、越权或证据缺失；
- 结果可复现；
- Prediction 能被测试结果直接支持。

证据不足必须返回 `inconclusive`，不能自动晋级。

## 10. Multi-Verifier Promotion Gate

门禁由独立验证器组成：

| Verifier | 作用 |
|---|---|
| Schema Verifier | 类型、未知字段、Hash 和大小限制 |
| Scope Verifier | 只修改允许组件和项目作用域 |
| Security Verifier | 路径、symlink、凭据、命令和权限 |
| Regression Verifier | Held-in/Held-out 配对结果 |
| Evidence Verifier | 每项结论可追溯到证据引用 |
| Cost Verifier | Token、耗时和候选预算 |
| Compatibility Verifier | Prompt、Manifest、Profile 和旧数据兼容 |
| Human Gate | 高风险或不确定决策的最终批准 |

验证器不得共享“全部通过”的单一布尔值；报告必须保留每个 Verifier 的独立结果。

## 11. 结构化 Playbook 生命周期

### 11.1 合并

- 按 Strategy ID 原子合并；
- 冲突时保留旧策略并拒绝覆盖；
- 同义或重复策略进入 dedup review；
- 渲染 Prompt 时按 component、scope、priority 确定性排序。

### 11.2 衰减与退休

- 长期未命中不等于无效，不自动删除；
- 有充分反证时标记 `retired`；
- 被更好策略替代时标记 `superseded`；
- 回滚后保留历史，不重新自动生成相同候选。

### 11.3 上下文预算

- 每个组件有 Token 预算；
- 超限时优先剔除 shadow、retired 和低优先级条目；
- 不允许用全文摘要重写整个 Playbook；
- 压缩只能生成新条目并保留来源映射。

## 12. Harness Updating 与 Harness Benefit 分离

OMR 必须分别回答：

1. Reasonix 是否能产生有效候选；
2. 当前任务模型是否能正确激活并遵守候选策略；
3. 候选是否真正改善任务结果。

因此 Episode 和 Evaluation 必须记录：

- Evolver 模型；
- Task Solver 模型；
- 策略是否被注入；
- 策略是否被命中；
- 策略是否被遵守；
- 遵守后结果是否改善。

不能把“Proposal 写得很好”当作 Harness Benefit。

## 13. 安全与 Reward Hacking 防线

- Evolution 目录不能包含或覆盖 Verifier；
- Candidate 不能修改 Fixture、预期输出、评分阈值或预算；
- 不允许提高模型等级、Token 上限或权限来伪造收益；
- 测试文件变化必须来自用户或独立治理流程；
- 所有评价输入和结果带 Hash；
- Candidate 工作目录必须隔离；
- 使用文件锁、CAS、原子写入和 Snapshot；
- 导入的经验永远是 pending；
- 安全失败 fail closed；
- Proposal 生成失败不能改变原始 Reasonix 任务退出码；
- 审计日志脱敏且不可由 Evolution Agent清理。

## 14. 预算、停止条件与人类触点

### 14.1 默认预算

- 每个 Pattern 最多 3 个候选；
- 每个候选最多 2 次修订；
- 每轮最多固定 Token 和 wall-clock；
- 同一 Pattern 连续 3 个候选失败后进入 cooling-off；
- 没有新增证据时不得重启搜索。

### 14.2 自动停止

- 收益连续两轮停滞；
- 候选与历史过度相似；
- Held-out 出现 blocking regression；
- 成本超过预算；
- 根因置信度不足；
- Verifier 结论冲突；
- 目标组件不在允许列表。

### 14.3 必须人工介入

- 修改 Profile 主体、路由或 Review 强制规则；
- 新增 Quality Fixture；
- 跨项目推广策略；
- Verifier 之间存在冲突；
- 候选影响安全、权限或兼容性；
- 进入源码/Skill 自进化研究阶段。

## 15. 文件布局建议

```text
.reasonix/omr/evolution/
├── episodes/
├── evidence/
│   ├── raw-refs/
│   └── analyses/
├── patterns/
├── candidates/
├── evaluations/
├── decisions/
├── lineage/
├── playbook/
│   ├── strategy-items/
│   └── rendered-overlay.md
├── archives/
│   ├── rejected/
│   ├── rolled-back/
│   └── inconclusive/
├── snapshots/
└── state.json

.reasonix/omr/trust/
├── component-registry.json
├── verifier-policy.json
├── budget-policy.json
└── trusted-keys/
```

`trust/` 由安装器或用户管理，Evolution Agent 只读。

## 16. CLI 设计

```bash
omr evolve diagnose [episode-id] --json
omr evolve patterns [pattern-id] --json
omr evolve candidates <pattern-id> --json
omr evolve validate <candidate-id> --json
omr evolve compare <candidate-id> --json
omr evolve approve <candidate-id> --json
omr evolve reject <candidate-id> --reason TEXT --json
omr evolve playbook list --json
omr evolve playbook show <strategy-id> --json
omr evolve playbook retire <strategy-id> --json
omr evolve lineage <candidate-id> --json
omr evolve budget --json
omr evolve doctor --json
```

所有命令必须支持稳定 JSON、明确 Schema Version 和稳定错误码。

## 17. 分阶段开发路线

### HE-00：Schema 与兼容层

- 冻结 EpisodeV2、CausalPattern、StrategyItem、CandidateProposal、EvaluationRun；
- v1 数据只读迁移和幂等转换；
- 未知字段、损坏记录和版本不兼容 fail closed。

验收：旧项目升级后现有 Overlay、Proposal 和历史不丢失。

### HE-01：证据归一化与因果诊断

- 确定性 Evidence Normalizer；
- Reasonix 严格 JSON Causal Analyzer；
- 支持证据、反例、外部原因排除和置信度。

验收：表层错误相同但根因不同的 Fixture 不再被错误聚类。

### HE-02：Component Registry

- 实现组件 Schema、允许路径、渲染器和验证器；
- 第一批只开放 `prompt.strategy`、`workflow.policy`、`review.policy` 和 `memory.playbook`。

验收：跨组件或未授权目标无法生成可批准 Candidate。

### HE-03：结构化 Playbook

- Strategy Item 增删、冲突、去重、排序、渲染和回滚；
- 从旧 `overlay.md` 迁移为单个 legacy Strategy Item。

验收：增量更新不会覆盖无关策略，Prompt 渲染确定且 Hash 稳定。

### HE-04：决策 Manifesto

- Proposal 必须包含根因、预测、风险和验收条件；
- 缺少可证伪预测时拒绝进入验证。

验收：每个生效策略都可追溯到证据和预测结果。

### HE-05：多候选与历史去重

- 每个 Pattern 生成最多 3 个互异候选；
- Candidate lineage、novelty、失败墓地和 cooling-off。

验收：重复或同义 Candidate 不重复消耗验证预算。

### HE-06：Held-in / Held-out 配对回放

- 固定快照、模型、预算、重复次数和顺序；
- Control/Candidate 双运行；
- 输出逐任务配对结果。

验收：只改善 Held-in 但破坏 Held-out 的 Candidate 必须拒绝。

### HE-07：Multi-Verifier Gate

- 独立 Schema、Scope、Security、Regression、Evidence、Cost、Compatibility Verifier；
- 不确定或冲突时转人工。

验收：任何一个 blocking Verifier 失败时零生效修改。

### HE-08：Harness Benefit 观测

- 记录策略注入、命中、遵守和结果；
- 按模型、Profile、Task Class 和 Harness Revision 分层；
- 区分 updating quality 与 benefit quality。

验收：报告不会把“已生成策略”误写成“任务质量提升”。

### HE-09：上下文生命周期

- Strategy Item Token 预算、冲突、退休、替换和确定性压缩；
- Context collapse 与 brevity bias 回归 Fixture。

验收：多轮进化后历史有效条目不会被无依据删除。

### HE-10：预算与自动停止

- 候选数、修订数、Token、耗时和 cooling-off；
- 无新证据、收益停滞和候选重复自动停止。

验收：无人值守运行具有明确成本上限。

### HE-11：跨项目与跨模型迁移评估

- 导入经验先进入 shadow；
- 在当前项目 Held-out 上验证后才能批准；
- 签名只证明来源，不证明适用性。

验收：外部经验包不能因为签名有效而直接生效。

### HE-12：长期健康与治理报告

- 维护性、兼容性、安全、成本和回滚率；
- 按 Harness Revision 输出长期趋势；
- 只报告统计事实。

验收：短期成功但长期回归的策略可被识别和退休。

### HE-13：实验性 Skill/Workflow Evolution

- 默认关闭；
- 只生成 Patch Proposal，不直接修改资产；
- 必须独立分支、完整测试、Security Review 和人工合并；
- 不自动修改 OMR Go 源码。

验收：实验失败不会影响当前安装和有效 Playbook。

### HE-14：Meta-Optimizer 研究

- 研究如何优化 Weakness Miner、Candidate Proposer 和 Context Curator；
- Optimizer 不能修改自身 Verifier、权限和预算；
- 仅在 HE-00～HE-12 积累足够真实数据后启动。

该阶段不进入近期产品版本。

## 18. 测试体系

每阶段必须先写失败测试，再实现最小代码。

### 18.1 单元与性质测试

- Schema、状态机、Hash、CAS、幂等；
- 路径穿越、symlink、大小和字段上限；
- Playbook 合并、去重、冲突和确定性渲染；
- Candidate lineage 和预算不变量；
- fuzz 非法 JSON 和边界输入。

### 18.2 故障注入

- 任意写入步骤失败自动恢复；
- Evaluation 中断可恢复；
- 并发批准、回滚和导入不会破坏状态；
- Verifier 不可用时 fail closed；
- Episode 不完整时不生成虚假 Pattern。

### 18.3 离线端到端

使用 Fake Reasonix/Fake Provider 覆盖：

```text
多条 Episode
→ 两种不同根因
→ 两个 Pattern
→ 每个 Pattern 三个候选
→ 配对回放
→ 一个通过、一个回归、一个 inconclusive
→ 人工批准
→ Playbook 生效
→ 观察
→ keep 或 rollback
```

### 18.4 真实验证

- 只在临时项目执行；
- 固定 Reasonix、OMR、模型和项目快照；
- 至少 3 次重复；
- 保留 Control/Candidate 原始脱敏证据；
- 不在只有单组或单次运行时声明质量提升。

## 19. 通用交付门禁

```bash
gofmt -w .
git diff --check
GOCACHE=/tmp/omr-gocache go test -count=1 ./...
GOCACHE=/tmp/omr-gocache go test -race ./internal/evolution/...
GOCACHE=/tmp/omr-gocache go vet ./...
GOCACHE=/tmp/omr-gocache go build ./cmd/omr
bash tests/docs_check.sh
```

另外必须执行：

- 临时目录 CLI Smoke；
- 敏感信息扫描；
- 安全 Review；
- Candidate Workspace 快照比较；
- Control/Candidate 配对回放；
- 升级、迁移、回滚和损坏恢复测试。

## 20. 成功标准

系统级成功标准：

- 每个策略修改都可追溯到证据、根因、目标组件和预测；
- 每个晋级 Candidate 都有 Held-in/Held-out 配对结果；
- Evolution Agent 无法修改 Trust Plane；
- 所有批准、拒绝、回滚和退休均可审计、幂等、可恢复；
- 多轮进化不会导致 Context Collapse；
- 成本存在硬上限；
- 失败和负结果不会丢失；
- 报告能区分 Harness Updating 与 Harness Benefit；
- 没有充分数据时明确输出 `inconclusive`。

效果成功标准：

- 在冻结模型和环境下，Held-in 目标失败减少；
- Held-out 不出现 blocking regression；
- Canary 100% 通过；
- Token、耗时和人工纠偏在预算内；
- 至少一个独立任务集能够复现收益；
- 不以单次成功或未配对结果宣称模型能力提升。

## 21. 推荐实施顺序

近期只执行：

```text
HE-00 → HE-01 → HE-02 → HE-03 → HE-04
→ HE-06 → HE-07 → HE-08 → HE-09 → HE-10
```

随后执行 HE-05、HE-11 和 HE-12。HE-13、HE-14 保持实验状态，必须单独评审。

最关键的产品里程碑不是“OMR 能修改更多东西”，而是：

> OMR 能够证明某个 Harness 修改针对一个真实根因，在冻结条件下改善目标任务，并且没有破坏未知任务；整个证据链可审计、可回滚、可复现。
