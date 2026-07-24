# 仅体验 OMR 的人工测试清单

本清单确认 OMR 已正确安装并能被 Reasonix 使用；它不是 OMR 相对原生 Reasonix 的效果证明。

## 1. 安装前与安装

```bash
cd /path/to/oh-my-reasonix
go build -o /tmp/omr-manual ./cmd/omr

cd <项目目录>
reasonix --version
/tmp/omr-manual version
/tmp/omr-manual init --project-dir . --dry-run
/tmp/omr-manual init --project-dir .
```

dry-run 应只计划项目目录内写入；不要静默接受用户 Prompt 冲突。

## 2. Doctor 与 Profile

```bash
/tmp/omr-manual doctor --project-dir .
/tmp/omr-manual doctor --project-dir . --json
/tmp/omr-manual profile list --project-dir .
/tmp/omr-manual profile list --project-dir . --json
```

通过标准：没有 blocking error；Prompt、Manifest、Profile Hash 和配置指向一致。让 Reasonix 分别使用 omr-explore、omr-research、omr-debug、omr-planner、omr-frontend、omr-git、omr-lsp 完成只读小任务，检查是否遵守只读边界。

## 3. Review、Session 与运行记录

```bash
/tmp/omr-manual hook doctor --project-dir . --json
/tmp/omr-manual task list --project-dir . --json
/tmp/omr-manual session list --project-dir . --json
```

如果当前 Reasonix 未提供对应机器接口，记录“不支持”，不要从人类可读 stdout 猜测结构化状态。

## 4. 升级和漂移

```bash
/tmp/omr-manual upgrade --project-dir . --dry-run
/tmp/omr-manual upgrade --project-dir .
/tmp/omr-manual doctor --project-dir .
```

要测试漂移检测，请先复制到临时目录，再修改临时目录中的 Prompt 或 Manifest，并确认 Doctor 报告问题；不要在真实项目中随意篡改配置。

## 5. 最小体验任务

依次发送给 Reasonix：

```text
只读探索当前项目，列出入口、测试入口和高风险区域，并引用文件路径。
```

```text
只读定位一个最值得优先修复的问题，先给根因证据和最小修复计划，不要改文件。
```

```text
对当前项目做一次安全 Review，只报告有文件证据的问题，并按严重级别排序。
```

记录是否使用预期 Profile、是否遵守只读约束、是否给出可复核证据，以及需要人工纠偏的次数。

## 6. 完成表

| 检查项 | 结果 | 证据位置 |
|---|---|---|
| init dry-run | ☐ | |
| init 安装 | ☐ | |
| doctor | ☐ | |
| Profile list | ☐ | |
| Explore / Research / Debug | ☐ | |
| Planner / Frontend | ☐ | |
| Git / LSP | ☐ | |
| Review 证据 | ☐ | |
| Session/Hook 机器接口 | ☐ / 不支持 | |
| upgrade/漂移检查 | ☐ | |

真实 Session 恢复、Hook 拦截和后台任务汇聚属于宿主联调项，需要对应 Reasonix 版本和用户观察客户端。
