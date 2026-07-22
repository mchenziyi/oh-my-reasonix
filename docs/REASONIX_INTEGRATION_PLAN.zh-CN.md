# OMR × Reasonix 集成计划

## 已验证的宿主能力

当前 Reasonix 分支已通过相关测试，具备：

- Native Todo 与未完成任务拦截；
- `complete_step` 证据校验，包括 `review` 证据；
- Session goal-state 持久化与 `--continue` / `--resume`；
- PreToolUse、PostToolUse、UserPromptSubmit、Stop 等 Hook；
- 内置 `review` Profile 与结构化 Review 回执。

OMR 复用这些能力，不维护第二套 Todo、Session 或事件状态。

## OMR 当前入口

```bash
omr session resume --project-dir <project>
omr session resume --project-dir <project> --copy
```

该入口只转发到 Reasonix 原生恢复参数。

## 下一阶段接口需求

以下能力需要 Reasonix 提供稳定的机器可读接口，OMR 才会继续扩展：

1. Session 列表、当前状态和未完成 Todo 的 JSON 查询；
2. Hook 能力与已启用 Hook 的 JSON 诊断；
3. 后台任务列表、状态和结果引用；
4. 失败原因与恢复链的结构化读取。

在这些接口稳定前，OMR 只提供 Prompt 约束、Doctor 校验、CLI 转发和离线质量报告，不复制宿主状态。
