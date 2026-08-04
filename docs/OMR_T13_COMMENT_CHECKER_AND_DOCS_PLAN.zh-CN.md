# OMR-T13：Comment Checker 与项目状态文档整理

> **Archived（已归档）**：本计划对应的工作已完成交付；本文保留作为审计与追溯依据，不再作为开发输入。当前能力与剩余宿主依赖见 [当前可用能力矩阵](OMR_CAPABILITY_MATRIX.zh-CN.md)。


> **状态：已完成** — 全部 5 个步骤（T13-01～T13-05）已执行完毕。
> Comment Checker 离线 CLI 已实现；文档状态已统一；运行时 Hook 已由后续 T14 交付（v2.0.1 起已通过 Reasonix v1.18 桌面端验证）。

> 用途：交给 Reasonix Agent 夜间自动执行。
> 范围：只修改 oh-my-reasonix 仓库；不修改 Reasonix 宿主仓库，不等待人工操作。
> 原则：先测试，后实现；只实现 OMR 能独立验证的部分，不猜测宿主 Hook、事件或 UI 接口。

## 一、目标

本任务包含两个独立交付项：

1. **Comment Checker 离线能力**：检查代码注释质量，输出确定性的报告；暂不实现 Reasonix 运行时拦截。
2. **文档状态整理**：统一 README、TODO、差距矩阵、CHANGELOG 和集成报告中的版本与完成状态。

## 二、明确边界

### 可以实现

- 纯 Go 的离线 Comment Checker；
- 基于文件快照或指定文件列表运行检查；
- 检测规则、稳定错误码、JSON/人类可读报告；
- Fixture、回放测试、脱敏和路径安全校验；
- README、TODO、差距矩阵、CHANGELOG 和集成报告状态同步。

### 明确不要实现

- 不读取 Reasonix 私有 Session、Task、Hook 文件；
- 不解析 `~/.reasonix` 私有目录；
- 不实现宿主 Hook 注册、实时事件监听或 Desktop UI；
- 不复制 Reasonix 的 Task/Session 状态机；
- 不把 OMR 合成的运行 ID 冒充 Reasonix Session ID；
- 不修改全局配置、API Key、PATH 或 Reasonix 二进制；
- 不修改 `artifacts/`、`omr-ab-b-meta.json` 和用户已有未跟踪文件。

## 三、T13-01：Comment Checker 规则与模型

先阅读现有质量基准、报告 Schema、Profile 资产和错误分类实现，避免重复造轮子。

建立最小领域模型，至少包含：文件路径与行号、规则 ID、严重级别、结果、脱敏摘要、总体状态和统计数量。

首批规则只做确定性、可离线验证的规则：

1. 注释中出现调试/临时标记（如 `TODO`、`FIXME`、`XXX`），但允许白名单；
2. 注释为空或只包含无意义占位文本；
3. 注释与相邻代码重复度过高时仅产生 `warning`，不得武断阻断；
4. 注释包含疑似密钥、Token 或密码格式时脱敏并产生 `blocking`；
5. 文件路径不在允许根目录内时拒绝检查。**默认以 `--project-dir` 为唯一允许根目录；**显式 `--allowed-roots` 时保留参数。相对 `--path` 按项目根解析，路径会通过 `EvalSymlinks` 解析符号链接，防止链接逃逸。

不要实现依赖大模型判断“注释是否有价值”的默认规则。

## 四、T13-02：CLI 与报告

增加最小 CLI 入口，例如：

```bash
omr comment-check --project-dir . --json
omr comment-check --project-dir . --path internal/foo.go --json
```

要求：默认只读；支持项目根目录和显式文件列表；JSON 字段稳定并含 `schema_version`；人类输出显示 finding、阻断数量和建议；出现 `blocking` 使用稳定非零退出码。

## 五、T13-03：Fixture 与测试

先写失败测试，再实现代码。至少覆盖：干净注释、TODO/FIXME、白名单、疑似密钥脱敏、路径穿越、二进制/超大文件跳过、重复运行稳定、JSON/人类输出一致、工作区快照不变。

禁止网络、模型调用和真实 Reasonix 客户端依赖。

## 六、T13-04：Profile 与文档接入

如现有资产结构适合，新增可选的 `omr-comment-checker` 只读 Profile；否则仅提供 CLI，不强行新增 Profile。同步 README、`OMR_TODO_LATEST.zh-CN.md`、差距矩阵、CHANGELOG 和质量报告说明。运行时 Hook/实时阻断明确写成等待 Reasonix 官方接口。

## 七、T13-05：文档状态整理

统一以下事实：T01～T12 已完成；INT-01～INT-05 已完成；INT-06 的机器接口已可用但真实客户端验证按实际报告填写；#6859 已合并；#6998 是 Reasonix Task Monitor/Desktop PR，不属于 OMR 核心完成项；Subagent 父子任务树等待官方父子事件/关联字段；Comment Checker 离线部分可完成，运行时 Hook 部分保持阻塞。

删除或改写过期表述，不删除历史报告；历史计划标注为历史计划/已完成。

## 八、自动执行门禁

```bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
go build ./...
go run ./cmd/omr version
```

另运行：

```bash
go run ./cmd/omr comment-check --project-dir <fixture> --json
```

如果没有修改前端或 Desktop，不要安装前端依赖或运行 Desktop 构建。

## 九、提交要求

建议拆成两个提交：

1. `feat: add offline comment checker`
2. `docs: reconcile omr status and roadmap`

提交前输出修改文件、规则和错误码、Fixture 数量、门禁结果、宿主阻塞项，并确认没有修改用户未跟踪文件。不得自行创建 Release、Tag、PR 或修改 Reasonix 官方仓库；完成后停在本地提交状态，等待 CTO Review。

## 十、完成标准

- Comment Checker 离线 CLI 可运行、默认只读、结果稳定；
- 发现和阻断规则有 Fixture 与测试证据；
- 敏感内容经过脱敏，不写入报告原文；
- OMR 文档的版本、阶段和阻塞状态一致；
- Comment Checker/CLI 定向测试、`go vet ./...`、`go build ./cmd/omr` 通过；
- 全量 `go test ./...` 若受执行环境禁止 `httptest` 本地端口监听影响，必须如实记录，不得伪报为通过；
- 没有把宿主 Hook、Task、Session 或 Desktop 能力伪造为已实现。
