# oh-my-reasonix 剩余开发任务书

## 1. 使用方式

本文件交给 Reasonix Agent 执行，开发目标仓库是当前
oh-my-reasonix 仓库。每完成一个任务，都必须先写回归测试，再修改最小代码，
运行自动化验证，最后更新本文件的状态。

不要修改 Reasonix 宿主仓库，也不要读取或解析 `~/.reasonix` 私有状态文件。

## 2. 当前已完成

### OMR 仓库内任务

- OMR-01：JSONC 配置读取；
- OMR-02：TOML → JSONC 迁移；
- OMR-03：安装、升级、卸载和回滚边界；
- OMR-04：质量 Fixture 扩展；
- OMR-05：质量报告 Schema；
- OMR-06：Profile 扩展调查，因宿主能力不足暂缓；
- OMR-07：Claude 配置只读导入；
- 基础 Doctor、Profile、Session resume、质量基准和 CLI Smoke。

### FIX 修正任务

- OMR-FIX-01 ✅ 撤回 Session 私有文件读取（session.go 已删除）
- OMR-FIX-02 ✅ 修正 Hook Doctor 语义（FAIL→UNSUPPORTED）
- OMR-FIX-03 ✅ JSONC 严格解析（拒绝多文档/重复键）
- OMR-FIX-04 ✅ 迁移失败可恢复（backup/no-dest/configDiff/permission）
- OMR-FIX-05 ✅ Claude 导入原子写入 + 权限恢复
- OMR-FIX-06 ✅ Claude 导入 JSON 校验 + Hook 转换标记
- OMR-FIX-07 ✅ 质量报告补充（zero metrics / 无合成 Session ID）
- OMR-FIX-08 ✅ CLI 一致性（现有实现已满足）
- OMR-FIX-09 ✅ 安装链路回归（CLI smoke + 全部测试通过）

### 暂不实现

- 直接读取 Reasonix Session 私有文件（FIX-01 已撤回）
- OMR 自己维护 Todo/Hook/Task/事件状态机
- `omr session list/status` 的伪 API
- 真实客户端 Session/Hook/后台任务验证
- 需要 Reasonix 新增 `--json` 接口的功能

以下任务是对现有实现的修正、补全和收尾，不要重复实现已完成项。

## 3. P0：移除不合规的宿主私有状态读取

### OMR-FIX-01：撤回 Session 私有文件读取

处理 `internal/reasonix/session.go` 及其调用方：

1. 删除或停用直接扫描 `~/.reasonix/projects` 的实现；
2. 移除依赖这些实现的 `omr session list/status/events/results`；
3. 保留已有的 `omr session resume` 和 `omr session export` CLI 转发；
4. 在 README 和命令帮助中说明 Session 查询等待 Reasonix 官方 JSON 接口；
5. 不破坏现有安装、Doctor、Profile 和 benchmark 命令。

验收：

- 仓库中不再出现读取 `~/.reasonix/projects` 的 OMR 生产代码；
- `go test ./...`、`go vet ./...` 和 CLI Smoke 通过；
- 不存在的 Session 不再被 OMR 伪造为“找不到”或空结果。

### OMR-FIX-02：修正 Hook Doctor 语义

1. Reasonix 没有 Hook 查询接口时，状态必须是明确的 `UNSUPPORTED` 或 `WARN`；
2. 不得显示误导性的 `FAIL)；
3. 不得声称 Hook 已被运行时强制执行；
4. 等宿主接口稳定后再增加真正的 Hook 转发检查。

验收：增加 JSON 和人类输出测试，确认 unsupported 不会被当成阻断错误。

## 4. P1：配置格式与迁移可靠性

### OMR-FIX-03：JSONC 严格解析

补充测试并修复：

- 拒绝一个 JSON 文档后追加第二个 JSON 文档；
- 拒绝重复键，行为与 TOML 加载一致；
- 保留字符串中的 URL、转义字符和注释文本；
- 错误信息包含文件、行和列；
- 不改变已有环境变量展开行为。

### OMR-FIX-04：迁移失败可恢复

补充并修复：

- TOML 重复键、未知字段和非法值必须在写入前阻断；
- 目标文件写入失败时不能留下半成品；
- 备份失败时不能写目标文件；
- 回滚必须恢复原内容和文件权限；
- `configDiff` 不得修改输入配置；
- 强制覆盖、冲突和幂等路径均有测试。

## 5. P1：Claude 导入安全性

### OMR-FIX-05：导入回滚与权限

修改 `internal/claude/import.go`：

- 使用原子写入；
- 区分“原文件不存在”和“原文件为空”；
- 回滚时恢复原文件权限；
- 创建目录失败或中途写入失败时恢复全部已写文件；
- 增加零字节文件、只读文件和中途失败测试。

### OMR-FIX-06：导入内容兼容性

- Claude Agent 导入后必须生成合法 Reasonix Skill/Profile frontmatter；
- MCP 导入前校验 JSON，错误时不写入；
- Hook 导入明确标记为“策略提示转换”，不得宣称等价于运行时 Hook；
- rules、skills、agents、mcp、hooks 的冲突和 dry-run 行为保持一致。

## 6. P1：质量与 CLI 收尾

### OMR-FIX-07：质量报告与 Fixture

- 为 `ValidateReport` 增加缺字段、类型错误、未知字段和版本不兼容测试；
- 确认所有 Fixture 可离线回放；
- 失败重试、停滞、成本、并发、Review 证据和恢复场景各至少有一个断言；
- 报告中不得把合成 ID 称为 Reasonix Session ID。

### OMR-FIX-08：CLI 一致性

统一以下命令的人类输出和 JSON 输出：

```bash
omr doctor
omr doctor --json
omr config validate
omr config validate --json
omr profile list
omr profile list --json
```

要求：错误退出码一致、字段名稳定、空列表不输出 null、帮助文本与实际命令一致。

### OMR-FIX-09：安装链路回归

使用临时目录覆盖：

- init、upgrade、uninstall；
- dry-run 零写入；
- manifest 缺失和 Hash 漂移；
- 外部资产目录；
- 用户配置冲突；
- 备份恢复；
- 重复执行幂等。

不得读取、覆盖或删除真实用户项目。

## 7. P2：文档与发布

### OMR-FIX-10：文档状态同步

同步 README、安装文档、差距矩阵和本任务书：

- 已完成、暂缓、阻塞三种状态必须一致；
- 明确 Session/Hook/Task 查询等待 Reasonix 官方接口；
- 删除“真实客户端已验证”等无证据表述；
- 每条命令示例必须可执行。

### OMR-FIX-11：发布前检查

增加或完善发布检查：

```bash
gofmt -w .
git diff --check
go test ./...
go vet ./...
./tests/cli_smoke.sh
```

检查未跟踪文件时必须保留用户已有的 `omr` 和 `.reasonix/`，不得自动纳入提交。

## 8. 暂不实现

以下项目不是当前 OMR 仓库内可独立完成的任务，必须保持暂缓：

- 直接读取 Reasonix Session 私有文件；
- OMR 自己维护 Todo、Hook、Task 或事件状态机；
- `omr session list/status` 的伪 API；
- 真实客户端 Session/Hook/后台任务验证；
- 需要 Reasonix 新增 `--json` 接口的功能。

等 Reasonix 宿主提供稳定公开接口后，再另开 OMR 适配任务。

## 9. 开发顺序

按以下顺序执行：

1. OMR-FIX-01、OMR-FIX-02；
2. OMR-FIX-03、OMR-FIX-04；
3. OMR-FIX-05、OMR-FIX-06；
4. OMR-FIX-07、OMR-FIX-08、OMR-FIX-09；
5. OMR-FIX-10、OMR-FIX-11。

只有命中真实 Reasonix 客户端测试门槛时才暂停并请求用户协助。
