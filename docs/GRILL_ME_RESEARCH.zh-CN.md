# Grill Me 集成调研

## 结论

Grill Me 更接近一个“方案质询/澄清” Agent Skill：在开始编码前，持续追问目标、约束、边界、失败场景和验收条件，帮助发现需求歧义。它不是 Reasonix 的底层运行时、Session 管理器或 MCP 服务。

当前建议：暂不默认集成，先保留为可选 Skill 设计项。

## 可能收益

- 减少需求误解和隐含假设；
- 在写代码前暴露边界条件、非功能要求和失败路径；
- 与 OMR Planner 的阶段拆分、验收条件形成互补；
- 适合复杂重构、架构设计、数据迁移和高风险改动。

## 与现有 OMR 的关系

- OMR Planner：把已明确的目标拆成执行步骤；
- Grill Me：在目标尚未明确时质询目标和决策；
- Review Profile：在实现后检查结果和证据。

因此它应作为可选的“需求澄清/方案质询” Profile 或 Skill，而不是替代 Planner、Review 或 Reasonix 原生 Todo。

## 集成风险

- 过度追问会拖慢简单任务；
- “持续追问”需要明确停止条件，否则可能形成对话循环；
- 如果做成 Hook，需要 Reasonix 提供稳定的任务开始/计划确认事件；
- 外部实现的具体提示词、许可证和维护状态需要先确认；
- 不应把第三方 Skill 原文直接复制进 OMR。

## 建议的最小试验

未来如要验证，只做离线可选 Profile：

1. 输入：任务目标、约束、已有方案；
2. 输出：澄清问题、假设、风险、待确认决策；
3. 用户确认后生成 Planner 可消费的结构化任务书；
4. 设置最多问题轮数和明确的完成条件；
5. 不修改文件、不调用 Hook、不启动后台任务。

## 暂不集成的原因

当前 OMR 的 T01～T09 和 INT-01～INT-05 已完成，真实客户端 INT-06 仍待验证。Grill Me 不阻塞 OMR 核心能力，优先级低于可选 Web/Docs MCP 和 Reasonix 机器接口联调。

## 参考

- Grill Me 公开介绍：<https://grillme.dev/>
- Agent Skills 说明：<https://agentskill.sh/@aiskillstore/grill-me>
- 社区技能目录（仅作线索，不作为 OMR 依赖）：<https://agentcookbooks.com/skills/grill-me/>
