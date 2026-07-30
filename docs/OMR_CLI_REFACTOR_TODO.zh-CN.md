# OMR CLI 工程化拆分 TODO

## 目标

在不改变 CLI 行为、输出格式和退出码的前提下，降低 `cmd/omr` 的维护成本，将命令实现按职责拆分，并为后续扩展建立清晰边界。

## 当前状态

- [x] `cmd/omr/main.go` 仅负责入口、命令分发和退出码映射
- [x] 提取版本命令至 `cmd/omr/version.go`
- [x] 提取 Comment Checker 至 `cmd/omr/comment_check.go`
- [x] 提取 Task 查询命令至 `cmd/omr/task.go`
- [x] 提取 Session 命令至 `cmd/omr/session.go`
- [x] 提取 Benchmark dispatcher/cache 至 `cmd/omr/benchmark.go`
- [x] 提取 Benchmark 评分辅助逻辑至 `cmd/omr/benchmark.go`
- [x] 提取质量 Benchmark 参数解析至 `cmd/omr/benchmark.go`
- [x] 提取 Hook Doctor 至 `cmd/omr/hook.go`
- [x] 提取 `run` 命令至 `cmd/omr/run.go`
- [x] 提取 JSON 报告写出逻辑至 `cmd/omr/output.go`
- [x] 提取资产加载入口至 `cmd/omr/install.go`
- [x] 提取 Config 迁移和 Prompt 文件校验至 `cmd/omr/config.go`
- [ ] 完成剩余命令模块拆分
- [ ] 建立统一 CLI 输出与错误边界
- [ ] 评估是否迁移到 `internal/cli`

## 分阶段任务

### CLI-REF-01：Session 命令拆分 ✅

- [x] 将 `runSession` 及其子命令移至 `cmd/omr/session.go`
- [x] 保持 `list/status/show/recovery/resume/export` 行为不变
- [x] 保持 JSON 字段、错误文本和退出码不变
- [x] 运行 `go test ./cmd/omr`、`go vet ./cmd/omr`、`go build ./cmd/omr`

### CLI-REF-02：Benchmark 命令拆分（进行中）

- [x] 将 `runBenchmark` 和缓存基准移至 `cmd/omr/benchmark.go`
- [x] 将质量评分门禁和结果读取辅助逻辑移至 `cmd/omr/benchmark.go`
- [x] 将质量 Benchmark 参数解析和主流程移至 `cmd/omr/benchmark.go`
- [x] 保持 replay、fixture、run-id、质量门禁参数兼容
- [x] 验证缓存基准回归测试
- [x] 保持质量 Benchmark 全部既有 flag 默认值和配置覆盖行为
- [ ] 运行质量基准和 CLI Smoke

### CLI-REF-03：Config 命令拆分（进行中）

- [x] 将迁移和 Prompt 校验移至 `cmd/omr/config.go`
- [x] 将内置 Profile 目录移至 `cmd/omr/config.go`
- [x] 将 Config Schema 输出移至 `cmd/omr/config.go`
- [x] 将 Config 校验 JSON 输出类型移至 `cmd/omr/config.go`
- [x] 将 Config 子命令路由移至 `cmd/omr/config.go`
- [x] 将 Profile JSON 输出类型移至 `cmd/omr/profile.go`
- [x] 统一 Profile JSON/人类输出的项目配置加载逻辑
- [x] 将 Profile 子命令路由移至 `cmd/omr/profile.go`
- [x] 将 Profile SKILL 元数据解析移至 `cmd/omr/profile.go`
- [x] 将项目级 Agent 配置覆盖移至 `cmd/omr/profile.go`
- [x] 将 Profile 分类和禁用状态组装移至 `cmd/omr/profile.go`
- [x] 统一项目专属 Profile 列表计算逻辑
- [x] 为 Profile 组装辅助逻辑增加回归测试
- [x] 将 `runConfigValidate` 主验证流程移至 `cmd/omr/config.go`
- [ ] 保持 TOML/JSON/JSONC 分发逻辑不变
- [ ] 覆盖 validate、migrate、schema 输出
- [ ] 验证迁移失败时零写入和回滚行为

### CLI-REF-04：Profile 命令拆分（进行中）

- [x] 将 Profile 列表路由、主输出流程和组装辅助逻辑移至 `cmd/omr/profile.go`
- [ ] 保持 Profile JSON Schema 和输出字段兼容
- [ ] 验证内置、项目级和禁用 Profile 的显示结果

### CLI-REF-05：安装与 Doctor 拆分

- [ ] 将 init/upgrade/uninstall 移至 `cmd/omr/install.go`
- [ ] 将 doctor 及其检查输出移至 `cmd/omr/doctor.go`
- [ ] 保持备份、回滚、漂移阻断和 JSON 输出不变
- [ ] 运行临时项目安装链路 E2E

### CLI-REF-06：Claude 与 Hook 拆分

- [ ] 将 Claude 导入命令移至 `cmd/omr/claude.go`
- [ ] 将 Hook 诊断移至 `cmd/omr/hook.go`
- [ ] 保持 dry-run、冲突检测、脱敏和 JSON 输出不变

### CLI-REF-07：统一输出边界

- [x] 提取 `cmd/omr/output.go`
- [x] 提取通用 flag 检测辅助逻辑
- [ ] 集中剩余 JSON 编码、人类输出和错误包装的公共逻辑
- [ ] 不改变现有命令的输出字段和错误码
- [ ] 为公共输出逻辑增加单元测试

### CLI-REF-08：评估 `internal/cli` 包迁移

- [ ] 盘点现有测试对 `package main` 未导出函数的依赖
- [ ] 设计最小 `App`/`Command` 接口和依赖注入边界
- [ ] 仅在迁移成本可控时建立 `internal/cli`
- [ ] 不为迁移而强行导出内部实现

## 每个任务的固定门禁

```text
gofmt -w <changed files>
git diff --check
go test -count=1 ./cmd/omr
go vet ./cmd/omr
go build ./cmd/omr
```

涉及质量基准、安装链路或其他内部包时，追加对应测试；完整 `go test ./...` 若受本机沙箱网络/监听限制失败，必须记录为环境问题，不得伪造通过。

## 暂不处理

- 不复制 Reasonix 的运行时状态机
- 不修改 CLI 的公开命令语法
- 不一次性重写全部测试
- 不引入复杂命令注册框架
- 不把无关文档、构建产物或未跟踪实验文件纳入提交

## 完成标准

当所有 CLI-REF-01 至 CLI-REF-07 完成，并且 CLI-REF-08 评估结论明确后，视为 OMR CLI 拆分阶段完成。
