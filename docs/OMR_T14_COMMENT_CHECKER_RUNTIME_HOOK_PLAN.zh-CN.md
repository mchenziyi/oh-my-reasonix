# OMR-T14：Comment Checker 运行时 Hook 开发计划

> 状态：待开发  
> 目标仓库：`oh-my-reasonix`  
> 依赖基线：Reasonix v1.18.0+  
> 状态：✅ 已完成并通过 Reasonix v1.18 桌面端验证。
>
> 原则：默认关闭、显式启用、只管理 OMR 自己的 Hook、失败可诊断、修改可回滚。

## 1. 背景

T13 已提供纯离线、确定性的 `omr comment-check` CLI，但不会自动参与 Agent 的开发流程。

Reasonix v1.18.0 已提供项目级原生 Hook：

- 项目配置：`.reasonix/settings.json`；
- 阻塞事件：`PreToolUse`；
- 工具匹配：锚定正则 `match`；
- stdin：一行 JSON payload；
- `exit 2`：阻断工具执行；
- `reasonix hook list/status --json`：只读诊断。

因此 T14 不实现第二套 Hook Runtime，只负责把现有 Comment Checker 安全接入 Reasonix 原生 Hook。

## 2. 产品行为

T14 实现一个默认关闭的“提交前注释质量门禁”：

1. 用户显式启用；
2. Reasonix 在执行 `bash` 工具前调用 OMR guard；
3. guard 只处理直接的 `git commit` 命令；
4. 非提交命令立即放行；
5. 提交命令触发项目级 Comment Checker；
6. 存在 blocking finding 时输出脱敏摘要并以退出码 2 阻断提交；
7. clean/warning/info 结果不阻断；
8. 用户可以查看状态、dry-run 或安全禁用。

本阶段不拦截普通文件写入，不解析 Reasonix 私有 Session/Task 文件，也不把 Comment Checker 复制进 Reasonix。

## 3. CLI 契约

新增：

```bash
omr hook comment-check enable --project-dir . --dry-run
omr hook comment-check enable --project-dir .
omr hook comment-check status --project-dir . --json
omr hook comment-check disable --project-dir . --dry-run
omr hook comment-check disable --project-dir .
omr hook comment-check guard --project-dir .
```

### 3.1 enable

- 默认先输出计划；
- 只有未传 `--dry-run` 时写文件；
- 仅修改项目级 `.reasonix/settings.json`；
- 保留所有未知顶层字段、其它事件和其它 Hook；
- 写入后提示“重启 Reasonix 后生效”；
- 重复 enable 必须 NOOP；
- 若存在同描述但内容不同的条目，返回冲突，不覆盖。

### 3.2 status

JSON 至少包含：

```json
{
  "schema_version": 1,
  "enabled": true,
  "owned": true,
  "settings_path": ".reasonix/settings.json",
  "event": "PreToolUse",
  "match": "bash",
  "command_available": true,
  "reasonix_visible": true,
  "issues": []
}
```

`status` 是只读操作。不得为了检查状态自动修复配置。

### 3.3 disable

- 只删除 OMR 精确拥有的条目；
- 保留同事件下其它 Hook 和全部未知字段；
- OMR 条目被用户修改时 fail closed，报告冲突；
- 重复 disable 必须 NOOP；
- 若移除后 `PreToolUse` 为空，可删除该空数组；
- 不因 OMR 删除而删除用户原本存在的 `hooks` 或 settings 文件。

### 3.4 guard

- 仅供 Reasonix Hook 调用；
- 从 stdin 读取一行 JSON，设置最大输入大小；
- 只接受 `event=PreToolUse`；
- `toolName` 不是 `bash` 时放行；
- 只识别明确、直接的 `git commit`；
- 不执行 payload 中的命令；
- 不把 `toolArgs`、命令正文或疑似凭据原样输出；
- 命中提交后直接调用 `internal/commentchecker`，不得递归启动另一个 `omr` 进程。

退出码：

| 场景 | 退出码 | Reasonix 行为 |
|---|---:|---|
| 非提交命令 | 0 | 放行 |
| clean / 只有 warning 或 info | 0 | 放行 |
| blocking finding | 2 | 阻断 |
| 已识别提交但扫描失败 | 2 | fail closed |
| payload 非法或事件不匹配 | 1 | 报告 Hook 错误但不伪造检查通过 |

## 4. Hook 配置

OMR 管理的唯一条目：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "match": "bash",
        "command": "/opt/homebrew/bin/omr hook comment-check guard --project-dir .",
        "description": "[oh-my-reasonix] Comment Checker before git commit",
        "timeout": 10000
      }
    ]
  }
}
```

约束：

- `description` 是所有权标记；
- `event`、`match`、`command`、`description`、`timeout` 必须全部匹配才视为 OMR 原始条目；
- 不添加 Reasonix 未公开的字段；
- 不写全局 `~/.reasonix/settings.json`；
- 启用时解析稳定安装的 `omr` 绝对路径，运行时不依赖 Reasonix Desktop 的 PATH；
- 配置中只写入经过验证的稳定绝对路径，不写 `go run` 临时路径；
- 使用 `go run` 的临时二进制不得宣称 Hook 可长期使用。

`enable` 前必须解析稳定、存在且可执行的 `omr` 绝对路径；`go run` 临时路径不得写入 Hook。不可用时阻止写入并给出安装提示，已有旧版相对命令在再次启用时迁移为绝对路径。

## 5. settings.json 合并规则

只接受可安全合并的顶层 JSON object。

1. 文件不存在：创建最小 canonical object；
2. 顶层不是 object：冲突；
3. `hooks` 不存在：新增；
4. `hooks` 不是 object：冲突；
5. `PreToolUse` 不存在：新增数组；
6. `PreToolUse` 不是数组：冲突；
7. 数组内未知条目：原样保留；
8. 已有完全相同 OMR 条目：NOOP；
9. 已有所有权标记但内容不同：冲突；
10. 不按 matcher、command 子串或数组位置猜测所有权。

允许重新格式化 JSON，但不能丢失未知字段或改变值。输出固定缩进和末尾换行，保证幂等。

## 6. 所有权、备份与回滚

扩展 OMR Manifest，增加可选 Hook 所有权记录；旧 Manifest 必须继续可读。

建议记录：

```yaml
hook:
  enabled: true
  settings_path: .reasonix/settings.json
  event: PreToolUse
  description: "[oh-my-reasonix] Comment Checker before git commit"
  entry_sha256: "..."
  base_file_sha256: "..."
  installed_file_sha256: "..."
```

写入流程：

1. 读取并验证 settings、Manifest 和路径；
2. 完成全部预检；
3. 创建项目内备份；
4. 原子写入 settings；
5. 原子写入 Manifest；
6. 任一步失败，恢复 settings 和 Manifest；
7. 不留下部分写入或临时文件。

`upgrade`：

- 默认不启用 Hook；
- Hook 已由 OMR 启用时才更新 OMR 条目；
- 用户修改 OMR 条目时冲突；
- 不覆盖其它 Hook。

`uninstall`：

- 自动移除未被修改的 OMR Hook；
- OMR Hook 被修改时阻止卸载并提示人工处理；
- 不删除用户 Hook；
- 失败时恢复全部文件。

## 7. 命令识别边界

MVP 只保证识别直接提交命令：

```text
git commit
git commit -m "message"
git -C path commit -m "message"
```

不在 T14 中实现完整 Shell Parser。不承诺识别：

- `sh -c "git commit"`；
- alias/function 包装；
- 动态拼接；
- `eval`；
- 含复杂管道或子 shell 的间接提交。

检测器必须是确定性的纯函数，并有表驱动测试。不得仅用 `strings.Contains("git commit")`，避免把 `echo "git commit"` 误判为提交。

## 8. Doctor

`omr doctor` 增加 Comment Checker Hook 检查：

| 状态 | 条件 |
|---|---|
| PASS | Manifest 声明启用、settings 条目与当前稳定绝对路径完全匹配、可执行文件有效、Reasonix `hook list/status` 可见 |
| WARN | 未启用；这是默认合法状态 |
| ERROR | Manifest 与 settings 漂移、条目损坏、命令不可用、Reasonix 报 invalid |
| UNSUPPORTED | Reasonix 版本低于 v1.18.0 或无 Hook 接口 |

Doctor 只诊断，不自动写入或修复。

## 9. 实现拆分

### T14-01：配置模型与纯合并器

新增独立包，例如：

```text
internal/commenthook/
  model.go
  settings.go
  settings_test.go
```

先实现纯内存 Plan：

- parse；
- enable merge；
- disable merge；
- ownership；
- stable JSON；
- conflict/noop/report。

不得在解析器内部写文件。

### T14-02：Guard 与提交识别

```text
internal/commenthook/
  guard.go
  guard_test.go
```

实现：

- 有界 stdin payload；
- 安全字段提取；
- 直接 `git commit` 检测；
- 调用 Comment Checker；
- 脱敏输出；
- 稳定退出码。

### T14-03：文件事务与 Manifest

实现：

- 路径边界和逐组件 symlink 检查；
- dry-run；
- 原子写入；
- 备份；
- 回滚；
- Manifest 可选字段向后兼容；
- upgrade/uninstall 集成。

### T14-04：CLI

扩展现有 `cmd/omr/hook.go`，不要把逻辑重新塞回 `main.go`：

- enable；
- status；
- disable；
- guard；
- JSON/人类输出；
- 稳定错误信息。

### T14-05：Doctor 与真实 Hook 诊断

- 复用现有 Reasonix Hook adapter；
- 校验项目 Hook 可见性；
- 区分“默认未启用”和“已启用但损坏”；
- 不解析 Reasonix 私有目录。

### T14-06：文档与手工验证方案

同步：

- `README.md`；
- `README.en.md`；
- `CHANGELOG.md`；
- `docs/OMR_TODO_LATEST.zh-CN.md`；
- `docs/OMR_VS_OMO_GAP_MATRIX.zh-CN.md`。

文档必须说明：

- 默认关闭；
- 仅阻断直接的 `git commit`；
- 需要重启 Reasonix；
- 需要可解析的稳定 `omr` 可执行文件；Reasonix Desktop 无需继承终端 PATH；
- 如何 dry-run、启用、诊断和禁用。

## 10. 自动化测试矩阵

至少覆盖：

### 配置

- 无 settings；
- 空 object；
- 未知顶层字段；
- 其它事件；
- 多个用户 PreToolUse Hook；
- 非 object hooks；
- 非 array PreToolUse；
- 相同 OMR 条目 NOOP；
- 所有权标记冲突；
- 用户修改 OMR 条目；
- JSON 损坏；
- JSON 稳定输出。

### 路径与事务

- settings 文件 symlink 越界；
- `.reasonix` 中间目录 symlink 越界；
- dry-run 零写入；
- settings 写入失败；
- Manifest 写入失败；
- upgrade 回滚；
- uninstall 回滚；
- 不删除用户 Hook；
- 重复 enable/disable 幂等。

### Guard

- 非 bash；
- 普通 bash；
- `echo "git commit"`；
- 直接 `git commit`；
- `git -C path commit`；
- clean；
- warning-only；
- blocking finding；
- 凭据脱敏；
- 扫描失败 fail closed；
- 非法 JSON；
- 超大 payload；
- 不修改项目文件。

### CLI/Doctor

- JSON Schema；
- 人类输出；
- 无法解析稳定 `omr` 可执行路径；
- Reasonix Hook 接口不可用；
- hook list 中可见；
- hook status invalid；
- disabled WARN；
- drift ERROR。

## 11. 自动门禁

每个子任务先写失败测试，再做最小实现。最终必须运行：

```bash
gofmt -w .
git diff --check
go test -count=1 ./internal/commenthook/...
go test -count=1 ./internal/commentchecker/...
go test -count=1 ./internal/install/...
go test -count=1 ./internal/doctor/...
go test -count=1 ./cmd/omr/...
go test -count=1 ./...
go vet ./...
go build ./cmd/omr
```

如果沙箱禁止 `httptest` 监听回环端口，必须记录为环境限制，并在允许本地端口的环境补跑；不得伪报通过。

## 12. 手工验收门槛

自动化完成后才需要用户协助一次：

1. 在临时项目执行 enable；
2. 重启 Reasonix；
3. `reasonix hook list/status --json` 确认条目可见；
4. 让 Agent 执行普通 bash，确认放行；
5. 创建 blocking comment；
6. 让 Agent 执行直接 `git commit`，确认被阻断；
7. 修复 comment 后再次 commit，确认放行；
8. disable 并重启 Reasonix，确认 Hook 消失；
9. 确认用户已有 Hook 未变化。

真实测试不得使用生产仓库、真实凭据或不可恢复的提交。

## 13. 明确不做

- 不修改 Reasonix 官方仓库；
- 不创建 OMR 自有 Hook Runtime；
- 不写全局 Reasonix 配置；
- 不自动启用；
- 不安装 shell、Node、Python 或其它运行时；
- 不内置完整 Shell Parser；
- 不拦截所有写工具；
- 不自动执行 git commit；
- 不读取 Reasonix 私有 Session/Task 状态；
- 不修改 API Key、PATH 或 Reasonix 二进制；
- 不将 Tmux、Task Monitor、父子任务树混入 T14。

## 14. 完成定义

只有同时满足以下条件，T14 才能标记完成：

1. 默认安装 OMR 不启用 Hook；
2. enable/disable/status/guard 全部实现；
3. 用户配置无损合并；
4. OMR 所有权可验证；
5. dry-run、幂等、冲突和回滚测试通过；
6. blocking finding 能稳定返回退出码 2；
7. 所有输出不泄露凭据或完整命令；
8. Doctor 能识别 disabled/enabled/drift/unsupported；
9. 全量自动门禁通过；
10. 真实 Reasonix 手工验收通过。

## 15. 交给 Reasonix Agent 的执行提示词

```text
请在 /Users/czy/Desktop/demo/oh-my-reasonix 中严格执行
docs/OMR_T14_COMMENT_CHECKER_RUNTIME_HOOK_PLAN.zh-CN.md。

先完整阅读任务书、现有 internal/commentchecker、internal/install、
internal/doctor、internal/reasonix 和 cmd/omr/hook.go，再开始修改。

按 T14-01 → T14-06 顺序执行，每一步先写失败测试，再做最小实现。
默认关闭，禁止自动启用；只修改项目级 .reasonix/settings.json；
必须保留用户已有字段和 Hook；只删除 OMR 精确拥有的条目；
所有写入必须支持 dry-run、冲突检测、备份、原子写入和回滚。

Guard 只识别明确、直接的 git commit，不实现完整 Shell Parser，
不执行 payload 中的命令，不输出完整 toolArgs、命令或凭据。
命中 blocking finding 返回退出码 2，其它规则按任务书执行。

不要修改 Reasonix 官方仓库，不读取私有 Session/Task 文件，
不要修改全局配置、PATH、API Key 或 Reasonix 二进制，
不要处理仓库现有无关未提交/未跟踪文件。

完成后运行任务书中的全部门禁，输出：
修改文件、测试结果、Hook 配置示例、退出码矩阵、
安全边界、仍需手工验证的步骤和剩余风险。
不要自行推送、创建 PR、Tag 或 Release；停下来等待 CTO Review。
```
