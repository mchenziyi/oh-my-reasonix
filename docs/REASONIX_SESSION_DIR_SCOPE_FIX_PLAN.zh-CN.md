# Reasonix v1.17.20 Session --dir 作用域修复计划

## 1. 问题摘要

Reasonix v1.17.20 对同一个 project Session 出现不一致行为：

~~~bash
reasonix session status session_4b0aaee180611e245dc818cc528c465e --json
# 返回 session.status + session

reasonix session status session_4b0aaee180611e245dc818cc528c465e +  --dir /Users/czy/Desktop/demo --json
# 返回 session_not_found
~~~

同样的问题出现在：

~~~bash
reasonix session list --json
# 可以看到 project sessions

reasonix session list --dir /Users/czy/Desktop/demo --json
# sessions: []
~~~

这会阻断 OMR 的 session list/status/recovery 与 task 作用域联调。

## 2. 预期行为

对同一个规范化项目根目录，以下命令必须返回一致的 Session 集合和状态：

- 不带 --dir 的默认查询；
- 带 --dir PROJECT_ROOT 的查询；
- PROJECT_ROOT 带或不带末尾斜杠的查询；
- 通过 GUI 创建的 Session 与 CLI 查询。

如果 --dir 是过滤条件，返回结果必须包含该目录下的 project Session；如果 --dir 是工作目录切换，则必须在所有机器接口中保持统一语义，并在 help/schema 中明确。

## 3. 调查范围

检查但不要读取或提交用户敏感内容：

1. CLI 参数解析和路径规范化；
2. GUI 创建 Session 时写入的 project root；
3. Session index 的 project key 生成；
4. symlink、末尾斜杠、相对路径和真实路径处理；
5. CLI/GUI install-id 与 IPC 查询上下文；
6. session list、status、recovery、task 的过滤逻辑是否一致。

禁止：

- 复制或修改 install-id；
- 读取并提交 API Key、完整 transcript 或私有数据库；
- 在 OMR 仓库中模拟宿主 Session 状态；
- 通过删除 Session 文件规避失败。

## 4. 最小复现 Fixture

测试 Fixture 必须自动创建临时项目和两个 project Session：

1. 创建 PROJECT_ROOT；
2. 通过官方 Session 创建路径生成 Session A；
3. 使用 session list/status 不带 --dir 查询；
4. 使用 --dir PROJECT_ROOT 查询；
5. 使用 PROJECT_ROOT/ 查询；
6. 比较 ID、scope、state、turns 和 recovered。

测试不得依赖用户当前的 ~/.reasonix 数据。

## 5. 验收标准

修复后必须满足：

- session list 的有/无 --dir 结果一致；
- session status 的有/无 --dir 结果一致；
- session recovery 的有/无 --dir 结果一致；
- task list 的 Session 过滤一致；
- 相对路径、绝对路径和末尾斜杠规范化一致；
- GUI 创建的 Session 可被 CLI --dir 查询；
- 不属于项目的 Session 不会被错误返回；
- 不同 install-id 或权限不足时返回明确错误；
- schema_version、错误码和退出码保持向后兼容。

## 6. 回归测试

至少增加：

- TestSessionListDirMatchesDefault;
- TestSessionStatusDirMatchesDefault;
- TestSessionRecoveryDirMatchesDefault;
- TestProjectRootNormalization;
- TestTrailingSlashNormalization;
- TestSessionScopeIsolation;
- TestTaskSessionFilterUsesNormalizedRoot;
- TestMachineIdentityErrorPreserved。

同时运行：

~~~bash
go test ./...
go vet ./...
gofmt -l .
~~~

## 7. 交付要求

请在 Reasonix 仓库中：

1. 先提交最小复现测试；
2. 再实现路径/作用域修复；
3. 增加 CLI 和 GUI 兼容回归；
4. 更新机器接口文档，明确 --dir 语义；
5. 生成脱敏测试报告；
6. 提交中文 PR；
7. 不修改 OMR 代码。

修复完成后，使用 Reasonix v1.17.20+ 重新验证 OMR INT-06。
