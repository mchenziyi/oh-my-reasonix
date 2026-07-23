# OMR-INT-01～INT-06：Reasonix 机器接口联调开发计划

## 1. 目标

在 Reasonix 官方机器接口已经合并并进入可用版本后，让 OMR 以只读方式消费宿主公开 JSON/JSONL 接口，完成 Session、Hook、Task、Recovery、事件流和结果汇聚的联调。

OMR 不维护第二套宿主状态机，不读取 Reasonix 私有文件，不修改宿主配置。

## 2. 前置条件

开始前必须确认：

1. Reasonix 官方机器接口 PR 已合并；
2. 当前使用的 Reasonix 版本支持以下命令和参数；
3. 所有接口返回 schema_version=1；
4. 参数错误、查询错误和空结果的退出码/错误码稳定；
5. 使用临时目录和 Reasonix Mock 完成自动测试；
6. 真实客户端验证只能放在 INT-06，不得混入单元测试。

接口基线：

~~~text
reasonix session list --json [--dir PATH]
reasonix session show|status <branch-id> --json [--dir PATH]
reasonix session recovery [<branch-id>] --json [--dir PATH]
reasonix hook list|status --json [--project-root PATH] [--home-dir PATH]
reasonix task list|show [<task-id>] --json [--dir PATH] [--session ID]
reasonix run --events-jsonl <task>
~~~

所有接口都必须按官方实际帮助输出核对，不得假设参数语义。

## 3. 总体安全边界

- 只调用公开 CLI 机器接口；
- 默认只读，不执行 Hook，不改变 trust，不恢复 Session，不修改 Task；
- 不读取 ~/.reasonix/projects、goal-state、events、lock 或桌面应用数据库；
- 不把 OMR 合成 run_id 冒充 Reasonix Session/branch ID；
- 不把空列表、接口不支持或查询失败伪装成成功；
- 保留脱敏边界：不输出 transcript、prompt、tool args/result、secret、绝对路径、PID、hostname；
- 所有 CLI 输出和 JSON 报告都要避免 API Key、环境变量值和用户项目内容泄漏。

## 4. INT-01：omr session list

### 实现

增加只读命令：

~~~bash
omr session list --project-dir <dir> [--binary <reasonix>] [--json]
~~~

要求：

- 调用 reasonix session list --json；
- 原样保留 schema_version 和官方脱敏字段；
- 输出 OMR 转发状态、命令、退出码和错误码；
- 稳定排序；
- 空列表仍输出 []；
- binary 不存在、版本不支持、JSON 损坏时明确失败。

### 测试

- Reasonix Mock 返回多个 Session；
- 空列表；
- 参数错误；
- binary 缺失；
- 非法 JSON；
- 脱敏字段检查；
- 人类/JSON 输出一致。

## 5. INT-02：omr session status/show

### 实现

增加：

~~~bash
omr session status <branch-id> --project-dir <dir> [--binary <reasonix>] [--json]
omr session show <branch-id> --project-dir <dir> [--binary <reasonix>] [--json]
~~~

要求：

- 明确使用 branch-id，不在 OMR 中重新命名为 Session ID；
- 转发官方状态、scope、turn、lifecycle、recovered 等字段；
- session_not_found、invalid_argument、查询失败分别保留；
- 不拼接 transcript、标题、绝对路径或私有事件内容；
- 不使用旧版 OMR 私有文件读取作为回退。

### 测试

- 存在/不存在 branch；
- 多个状态；
- recovered 状态；
- 错误码和退出码；
- JSON Schema 与脱敏断言；
- 旧版本不支持时 Doctor/CLI 明确提示。

## 6. INT-03：omr hook doctor

### 实现

增加：

~~~bash
omr hook doctor --project-dir <dir> [--home-dir <dir>] [--binary <reasonix>] [--json]
~~~

要求：

- 只调用 reasonix hook list/status；
- 不执行 Hook，不改变 trust；
- 检查 OMR 所需事件、matcher、scope 和 active/untrusted 状态；
- 报告 source 统计和缺失项；
- 将 unsupported、inactive、untrusted、parse error 区分；
- 只输出官方脱敏字段。

### 测试

- 全部 Hook 正常；
- 缺失 Hook；
- inactive/untrusted；
- source 统计；
- Mock 返回错误；
- 人类/JSON 输出一致；
- 命令不会被执行的安全测试。

## 7. INT-04：omr task list/show

### 实现

增加：

~~~bash
omr task list --project-dir <dir> [--session <id>] [--binary <reasonix>] [--json]
omr task show <task-id> --project-dir <dir> [--session <id>] [--binary <reasonix>] [--json]
~~~

要求：

- 只读转发 task/subagent ID、parent session、type、status、时间和 artifact 完整性；
- 不输出 label、参数、路径、输出正文或 mutation receipt；
- 正确处理 task_not_found、task_ambiguous；
- 不复制后台任务状态机；
- 多 Session 查询必须保留官方 session 关联。

### 测试

- 空任务；
- running/succeeded/failed；
- task_not_found；
- task_ambiguous；
- session 过滤；
- artifact 完整性；
- 脱敏字段和稳定排序。

## 8. INT-05：Recovery、事件流和结果汇聚

### 实现

### 8.1 Recovery

增加：

~~~bash
omr session recovery [<branch-id>] --project-dir <dir> [--binary <reasonix>] [--json]
~~~

只转发状态、任务/失败/pending/in-flight 计数和时间；不输出 tool、subject、args、failure excerpt 或 proposal。

### 8.2 结构化事件流

为 OMR 质量运行或只读观察增加事件转发适配：

~~~bash
reasonix run --events-jsonl <task>
~~~

要求：

- 不复用富文本 stream-json；
- 按行解析 JSONL；
- 保留 event kind、序号、opaque tool ID/name、状态、token 计数和稳定类别；
- 末行 run_done；
- 非法行、缺失 run_done、乱序序号和超大行明确失败；
- 不写入 Prompt、tool args/result、reasoning、approval 文本或 compaction summary。

### 8.3 结果汇聚

- 只消费脱敏事件和 artifact 完整性；
- 生成 OMR 报告时使用合成 run_id；
- 显式标注 source=reasonix_machine_interface；
- 不把结果引用当作文件路径读取；
- 与 T05 报告字段兼容，缺失字段使用 unknown/null，不使用虚假零值。

### 测试

- Recovery 各状态；
- 合法事件流；
- 空事件流；
- 非法 JSONL；
- 缺失 run_done；
- 事件顺序错误；
- 脱敏字段；
- 结果汇聚与质量报告 Schema；
- Reasonix 接口错误不会产生部分成功报告。

## 9. INT-06：真实客户端验证

该阶段必须暂停自动开发并请求用户协助。

### 用户操作

1. 安装包含官方机器接口的 Reasonix 版本；
2. 启动 Reasonix 客户端；
3. 在指定临时项目运行一个可观察任务；
4. 让任务产生 Session、Hook、Task、Recovery 或事件流状态；
5. 按 OMR 命令逐项执行查询；
6. 观察客户端与 OMR 输出是否一致。

### 验收记录

- Reasonix 版本和 commit；
- 使用的项目目录；
- 每条命令、退出码和脱敏输出；
- 客户端可观察行为；
- OMR 输出；
- 不一致项和后续修复；
- 不上传不必要的用户项目内容。

在用户未启动客户端或未授权读取项目内容前，INT-06 必须标记 BLOCKED，不能声称通过。

## 10. 通用适配器要求

建议集中实现一个最小 Reasonix CLI 适配器，负责：

- binary 解析；
- 版本/能力探测；
- 命令执行；
- stdout/stderr/退出码；
- JSON/JSONL 解析；
- 稳定错误映射；
- 脱敏校验。

适配器不得持久化宿主状态，不得自动重试改变状态的命令，不得执行 shell 拼接。参数必须使用 argv 数组传递。

## 11. 自动门禁

每个 INT 阶段完成前执行：

~~~bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
go build ./...
~~~

使用 Mock 执行：

~~~bash
go test ./... -run 'Reasonix|Session|Hook|Task|Recovery|Event'
~~~

临时目录 Smoke：

~~~bash
go run ./cmd/omr session list --project-dir <temp-project> --json
go run ./cmd/omr session status <branch-id> --project-dir <temp-project> --json
go run ./cmd/omr hook doctor --project-dir <temp-project> --json
go run ./cmd/omr task list --project-dir <temp-project> --json
~~~

所有测试必须断言：

- 只读；
- 脱敏；
- 错误码；
- 空结果；
- 稳定 JSON；
- 无私有文件回退。

## 12. 提交策略

建议按以下提交拆分：

- INT-01: session list；
- INT-02: session status/show；
- INT-03: hook doctor；
- INT-04: task list/show；
- INT-05: recovery/events/results；
- INT-06: real client validation and docs。

每个提交只包含对应适配器、命令、测试和文档。INT-06 的人工验证记录不得包含敏感项目内容。

## 13. 完成标准

- INT-01～INT-05 自动测试全部通过；
- INT-06 有真实客户端验证记录，或明确标记 BLOCKED；
- 不读取 Reasonix 私有存储；
- 不复制宿主状态机；
- 不泄漏 Prompt、路径、参数、输出、凭证或主机信息；
- OMR 报告与 Reasonix 官方 schema/version 兼容；
- README、安装文档、Doctor 和差距矩阵同步更新。

## 14. 交给 Reasonix Agent 的执行指令

请严格按 INT-01 → INT-05 顺序实现，每一步先写 Mock/失败测试，再做最小 CLI 适配；INT-06 只有在需要真实客户端时才请求用户协助。完成后输出修改文件、接口版本、测试命令及结果、脱敏审计结果、未兼容能力和是否需要人工验证。禁止读取私有文件、猜测接口或伪造真实客户端通过。

## 15. Reasonix 版本与分支策略

- Reasonix PR #6762 已合并：需要查看其最终代码时，直接同步 `main-v2`，不要继续使用 #6762 的旧 PR 分支。
- Reasonix PR #6859 尚未合并：INT-01～INT-06 联调和测试必须使用 #6859 的最新 PR commit；不得用当前 `main-v2` 代替。
- 每次测试记录：Reasonix 分支名、commit SHA、构建命令和 CLI `--version` 输出。
- #6859 合并后，重新基于最新 `main-v2` 至少复跑 INT-01～INT-05 的 Mock/Smoke 和兼容性检查。
- 若 #6859 分支发生 rebase 或新增 commit，先重新同步并记录新的 SHA，再继续测试；不要把旧二进制结果当作最新结果。
