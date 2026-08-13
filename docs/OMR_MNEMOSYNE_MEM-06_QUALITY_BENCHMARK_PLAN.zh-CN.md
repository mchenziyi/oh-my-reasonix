# OMR Mnemosyne MEM-06：Memory Quality Benchmark

- 阶段：MEM-06
- 状态：✅ 配对 Fixture/Report 自动化与只读 CLI 已实现并通过自动门禁；真实 Reasonix 临时项目配对 Benchmark 尚未执行
- 前置：MEM-01～MEM-05 的自动门禁通过；真实 Reasonix Desktop 联调仍需用户协助
- 目标：只报告 Mnemosyne 对检索、读取/采用和下游任务的事实指标，不宣称通用模型能力提升

## 一、固定实验协议

每个 Fixture 必须固定：

- Reasonix/OMR 版本、Profile、模型、温度和工具权限；
- 任务文本、项目快照、Memory Generation、Policy、重复次数和随机种子；
- Oracle/评分器版本、判定规则和最低样本量；
- 配对标识，使 `Reasonix + Mnemosyne` 与 `Reasonix without Mnemosyne` 使用同一任务世界。

## 二、指标分层

1. Retrieval：Recall@K、Miss Rate、Scope/Hash 错配率、unavailable 率；
2. Reading/Adoption：读取命中率、错误记忆采用率、冻结记忆误用率、Evidence 回执完整率；
3. Downstream Task：任务成功率、回归率、耗时、Token/成本和人工 Oracle 结果。

样本不足、无法配对、评分器错误或事实缺失统一输出 `insufficient_evidence`；不进行无依据的统计显著性或模型能力结论。

## 三、执行边界

- Benchmark 只读输入 Fixture 和固定 Generation，不改 Memory Fact、CURRENT、Lifecycle 或 Overlay；
- 报告脱敏，不输出完整 Prompt、命令、思考、凭据或项目路径；
- 失败不自动冻结或回滚记忆；若需修复，生成独立 Repair/Promotion 计划；
- 真实客户端联调前不读取正式项目、不使用正式 API Key、不创建 Release/Tag。

## 四、验收与人工联调

自动部分先验证 Fixture Schema、配对键、指标计算、报告确定性和脱敏。自动门禁通过后，用户在临时项目执行一组固定任务，分别运行有/无 Mnemosyne 两组，检查 Scope 隔离、Recall 回执、冻结读取、回滚预览和报告中的 `insufficient_evidence` 标记。

只读 CLI 入口：

```bash
omr memory benchmark paired \\
  --paired-fixture /tmp/omr-mem06/paired-fixture.json \\
  --json
```

该命令只读取 Fixture 并输出报告；`--output <path>` 可将 JSON 写入指定文件，不创建或修改 Memory Fact、CURRENT、Lifecycle 或 Overlay。

2026-08-14 进程级 CLI smoke：使用隔离临时目录中的人工构造三案例 fixture 执行 `omr memory benchmark paired --json`，稳定返回 `protocol_only=true`、`evidence_status=sufficient`、`case_count=3`，两组 Recall/Reading/Downstream 指标均可重现。该 smoke 只验证协议与 CLI，不替代真实 Reasonix 模型 A/B 测试，也不宣称质量提升。

## 五、交给 Reasonix Agent 的执行提示词

```text
执行 OMR Mnemosyne MEM-06。先读取本计划、现有 benchmark.go、MEM-03C Recall、MEM-04A/04B 回执、MEM-05A～D
计划和 Architecture v1 Benchmark 约束。

先做 Schema Gate：复用现有 Fixture/BenchmarkReport，不新增第二事实源，不把单机离线测试冒充真实 A/B。严格 TDD，
实现固定版本/模型/任务/种子/Oracle 的配对报告，分别计算 Retrieval、Reading/Adoption、Downstream Task 三层指标。
无法配对或样本不足时输出 insufficient_evidence；报告只含统计事实，脱敏且确定性，不改 Fact/CURRENT/Lifecycle/Overlay，
不调用未经批准的网络/API。运行完整 Go 门禁、docs_check、review/security review；真实客户端联调前不要提交正式项目数据、
Release 或 Tag。
```
