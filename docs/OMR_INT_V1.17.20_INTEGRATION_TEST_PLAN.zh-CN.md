# OMR × Reasonix v1.17.20 联调验证方案

> 交给 Reasonix Agent 执行。
> 目标：验证 OMR 的 INT-01～INT-05 是否与 Reasonix v1.17.20 官方机器接口兼容，并为 INT-06 生成真实客户端验证步骤。
> 原则：只验证和修复 OMR 适配层，不读取 Reasonix 私有文件，不复制宿主状态机。

## 1. 前置检查

先记录 Reasonix 二进制路径、版本、OMR Git commit、项目路径、Go 版本和工作区状态。

必须确认版本输出为 reasonix v1.17.20。版本不匹配时停止联调并报告。

逐项读取官方帮助，确认实际参数，不得猜测：

~~~bash
reasonix session --help
reasonix session list --help
reasonix session status --help
reasonix session recovery --help
reasonix hook --help
reasonix hook list --help
reasonix hook status --help
reasonix task --help
reasonix task list --help
reasonix run --help
~~~

## 2. 安全边界

- 只调用公开 CLI 机器接口；
- 默认只读，不执行 Hook、不改变 trust、不恢复 Session、不修改 Task；
- 不读取 ~/.reasonix 下的私有数据库、events、lock 或 goal-state；
- 不把 OMR 合成 run_id 冒充 Reasonix branch/session ID；
- 不输出 transcript、prompt、tool 参数、tool 结果、reasoning、密钥、PID 或绝对路径；
- 接口不支持、空列表、查询失败和 JSON 损坏必须分别报告；
- 不修改 OMR 生产代码，除非先产生可复现失败并获得用户确认。

## 3. 自动化验证

在 OMR 仓库执行：

~~~bash
gofmt -l .
git diff --check
go test ./...
go vet ./...
~~~

建立临时目录和 Reasonix Mock，验证：

- reasonix session list --json
- reasonix session status <id> --json
- reasonix session recovery [<id>] --json
- reasonix hook list --json
- reasonix hook status --json
- reasonix task list --json
- reasonix task show <id> --json
- reasonix run --events-jsonl <path> <prompt>

记录每个接口的退出码、schema_version、JSON 字段和错误行为。

## 4. INT-01/02：Session 联调

执行：

~~~bash
omr session list --project-dir <temp-project> --json
omr session status <branch-id> --project-dir <temp-project> --json
omr session recovery <branch-id> --project-dir <temp-project> --json
~~~

验证：

- OMR 使用官方 CLI，不读取私有文件；
- branch/session ID 原样保留；
- 空列表输出合法空数组；
- 不存在的 ID 保留 not_found 语义；
- schema_version、lifecycle、turn、recovered 和计数不丢失；
- binary 缺失、版本不支持、非法 JSON 有明确错误；
- 人类输出和 JSON 输出语义一致。

## 5. INT-03：Hook 联调

执行：

~~~bash
omr hook doctor --project-dir <temp-project> --home-dir <temp-home> --json
omr hook doctor --project-dir <temp-project> --home-dir <temp-home>
~~~

验证：

- 同时查询官方 hook list/status；
- 不执行 Hook，不修改 trust；
- 区分 active、inactive、untrusted、unsupported 和查询错误；
- JSON 始终包含稳定的 status 字段；
- 错误写入 Error/Unavailable，不被吞掉；
- 输出不包含敏感参数或绝对路径。

## 6. INT-04：Task 联调

执行：

~~~bash
omr task list --project-dir <temp-project> --json
omr task list --project-dir <temp-project> --session <session-id> --json
omr task show <task-id> --project-dir <temp-project> --json
~~~

验证：

- task 与 session 的官方关联保留；
- running、succeeded、failed、empty 状态均可表达；
- task_not_found、task_ambiguous 不被转成成功；
- 不输出 prompt、tool args/result、label、路径或 mutation receipt；
- 多 Session 查询不会混淆结果；
- 稳定排序和 JSON Schema 通过。

## 7. INT-05：事件流和结果汇聚

先直接验证 Reasonix：

~~~bash
reasonix run --events-jsonl <temp-events.jsonl> <prompt>
~~~

再验证 OMR：

~~~bash
omr run --project-dir <temp-project> --events-jsonl <temp-events.jsonl> <prompt>
~~~

检查 events 文件是否生成、JSONL 是否逐行可解析、seq 是否单调、是否存在唯一 run_done、token 汇总是否一致，以及非法 JSON、超大行、乱序和缺失 run_done 是否明确失败。

失败时不得生成部分成功报告。事件中不得出现 prompt、reasoning、tool 参数、tool 结果或 secret。OMR 报告标记 source=reasonix_machine_interface。

必须保存独立命令、stdout、stderr、退出码和 events 文件摘要，不能只引用 UI 观察。

## 8. Mock 回归矩阵

覆盖以下情况：

| 场景 | 预期 |
|---|---|
| 合法 JSON | 解析成功 |
| 空列表 | 输出空数组/空结果 |
| 非法 JSON | 明确 parse error |
| 命令不存在 | 明确 unavailable |
| 版本不支持 | 明确 unsupported |
| not_found | 保留查询失败语义 |
| 权限错误 | 分类 infrastructure |
| 缺失字段 | 输出 unknown/null，不填假零值 |
| 事件无 run_done | evidence incomplete 或失败 |
| 事件序号乱序 | 明确校验失败 |

## 9. INT-06：真实客户端验证（需要用户协助）

自动验证全部通过后，再请求用户协助：

1. 启动 Reasonix v1.17.20 客户端；
2. 在临时项目启动一个可观察任务；
3. 让客户端产生 Session、Hook、Task 和事件状态；
4. 不关闭占用该 Session 的窗口；
5. 用户告知已启动、已发送或已中断等状态；
6. Agent 在另一个终端运行 OMR 查询命令；
7. 对比 Reasonix UI 与 OMR JSON 输出；
8. 用户手工恢复一次中断 Session；
9. 再次查询 status、recovery、task 和 events；
10. 记录 UI 与 CLI 是否一致。

客户端没有产生对应状态时，必须标记 not_observed，不能伪造通过。

## 10. 报告要求

生成：

~~~text
docs/OMR_INT_V1.17.20_INTEGRATION_REPORT.zh-CN.md
~~~

报告必须包含版本和帮助输出摘要、INT-01～INT-05 每个接口的命令/退出码/结果、Mock 回归矩阵、事件流证据摘要、脱敏检查、失败分类、INT-06 用户操作与 UI/CLI 对照结果，以及仍需上游配合的事项。

不得把 Reasonix 宿主能力误归因于 OMR。

## 11. 交付门禁

完成后执行：

~~~bash
gofmt -l .
git diff --check
go test ./...
go vet ./...
~~~

如果发现代码缺陷，先提交最小复现测试和修复建议；本轮默认只生成联调报告，不自动修改生产代码。
