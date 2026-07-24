# OMR 两天无人值守自动开发计划

> 执行者：Reasonix Agent。
> 执行周期：连续 48 小时以内。
> 目标：在不需要用户人工介入的前提下，完成 OMR 的可自动化收尾和产品化优化。
> 当前基线：Reasonix v1.17.20，OMR main 最新提交。

## 1. 不可触碰的边界

自动执行期间禁止：

- 修改 Reasonix 宿主代码；
- 访问、打印或提交 API Key、Cookie、完整模型敏感输出；
- 修改全局 PATH、shell 配置或系统权限；
- 读取或改写 Reasonix 私有数据库、Session 文件和桌面应用数据；
- 删除用户文件、使用 git reset --hard、force push 或重写远程历史；
- 执行需要用户点击、GUI 观察或人工确认的任务；
- 提交 artifacts、临时日志、模型完整输出和 omr-ab-b-meta.json；
- 因测试失败反复破坏性重试。

所有修改必须限制在 OMR 仓库，使用最小变更和可回滚提交。

## 2. 自动执行原则

每个任务严格执行：

1. 读取当前代码和相关测试；
2. 先新增回归测试或 Fixture；
3. 做最小实现；
4. 运行格式化、测试、静态检查；
5. 检查 diff 和敏感内容；
6. 更新文档；
7. 创建独立提交；
8. 推送到 main；
9. 记录任务结果和提交哈希。

单个任务连续 3 次无法通过门禁时，停止该任务，保留失败证据，继续下一个独立任务。

## 3. 统一门禁

每个任务至少执行：

~~~bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
~~~

涉及 CLI 时额外执行 doctor/profile JSON Smoke；涉及质量基准时额外执行离线 Fixture 回放，不调用真实模型服务。

## 4. P0：基线和工作区治理（第 0～2 小时）

### AUT-00 环境冻结

记录 OMR commit、Go/Reasonix 版本、当前 Tag、测试包数量和当前未跟踪文件。

验证 main 工作区无已跟踪脏改动。不得删除已有未跟踪 artifacts。

### AUT-01 测试稳定性

连续运行三次：

~~~bash
go test ./... -count=1
~~~

端口、网络或沙箱问题分类为环境阻断，不修改生产逻辑绕过。

## 5. P1：配置体验修复（第 2～8 小时）

### AUT-02 config validate 语义统一

验证安装后的临时项目在没有 OMR config.toml 时的行为。

目标：

- 无配置文件不被误报为安装损坏；
- JSON 和人类输出语义一致；
- 区分未配置、配置有效、配置非法；
- 不改变全局配置；
- 迁移和卸载兼容。

优先方案：返回 valid=true、configured=false；若设计必须报错，则更新命令帮助和 README，明确该命令只校验已存在配置。

增加缺失配置、空配置、合法配置、非法配置、JSON 输出和 dry-run 不写入测试。

## 6. P1：CLI 维护性（第 8～20 小时）

### AUT-03 拆分 CLI 入口

只在不改变行为的前提下，将 cmd/omr/main.go 中可独立命令拆到内部包。

优先拆分 hook、session、task、run、config/profile。命令、退出码和 JSON 字段必须兼容。

每次只拆一个边界；若拆分风险超过收益，保留现状，只提取测试辅助函数，不强行重构。

## 7. P1：README 和首次使用体验（第 20～28 小时）

### AUT-04 README 产品化

完善 README：

- OMR 定位和解决的问题；
- 一分钟安装；
- init/upgrade/doctor/profile/run 示例；
- OMR Profile 表；
- OMR 与 Reasonix 原生能力边界；
- 安装、备份、升级、回滚、卸载；
- TOML/JSON/JSONC 配置；
- Claude 导入；
- v1.17.20 机器接口兼容状态；
- 常见错误和排查；
- 明确 INT-06 需要真实客户端，不能伪造已完成。

所有命令示例必须在临时目录实际执行。

## 8. P1：联调回归和报告整理（第 28～34 小时）

### AUT-05 v1.17.20 适配回归

使用 Mock 和本地 CLI 验证 session list/status/recovery、hook list/status、task list/show、events-jsonl、v1.17.20 event schema、旧格式兼容、非零退出事件落盘、事件脱敏、sequence、run_done 和 token 汇总。

只把真实运行通过的接口标记为通过。INT-06 保持 pending。

### AUT-06 报告一致性检查

自动检查版本、测试数量、变更清单和旧结论清理；不得残留过期数字、绝对路径、API Key 或完整模型输出。

## 9. P2：质量和发布准备（第 34～42 小时）

### AUT-07 质量 Fixture 和离线基准

只扩展离线 Fixture，覆盖配置漂移、事件流失败、失败事件落盘、配置回滚、Profile 冲突和 JSON 稳定性。运行 benchmark replay，不调用真实模型。

### AUT-08 发布检查

检查 README、CHANGELOG/Release Notes、Tag、CI、go.mod/go.sum、构建命令和安装/卸载 Smoke。

如最新提交未包含当前 Tag，生成 v1.1.3 发布建议，但不要自动覆盖已有 Tag。

## 10. 可选任务

仅在 P0～P1 全部完成后，按顺序选择文档归档、CLI 快照测试、配置迁移边界、Profile 路由诊断增强、Comment Checker 设计、Grill Me 调研和 Tmux 宿主需求文档。

核心任务未完成时不要开始可选任务。

## 11. 自动提交规则

提交标题使用 docs、test、fix、refactor 或 chore 前缀。每个提交只包含一个逻辑任务。

推送前确认：

~~~bash
git diff --cached --check
git status --short
git log -1 --oneline
~~~

明确排除 artifacts/、omr-ab-b-meta.json、临时目录和未脱敏事件日志。

## 12. 最终交付报告

生成：

~~~text
docs/OMR_AUTONOMOUS_2DAY_EXECUTION_REPORT.zh-CN.md
~~~

报告必须包含任务完成/跳过/阻断状态、提交哈希、测试门禁、失败重试、未提交文件、Tag 状态和仍需用户协助的事项。

最终状态只能使用 completed、blocked、skipped、failed 四类。不得把 INT-06 或 GUI 行为伪造为完成。

两天结束后停止自动开发，不自行扩大范围。
