# OMR Mnemosyne MEM-03C-04：Reasonix 接入与真实联调计划

- 状态：🟡 Composite Generation 已实现，待 CLI/Profile 接入
- 前置：MEM-03C-01～03 已实现并推送
- 目标：让 Reasonix 通过稳定的 OMR CLI 与 `omr-memory` Profile 使用固定世界 Episodic Recall

## 一、实现前 Gate：单 CURRENT 冲突

FactStore 只有一个 `CURRENT`。MEM-03B Memory Librarian 需要 CURRENT 指向包含
`wiki/index.md` 与 `state/index-tree.json` 的 OKF Generation；MEM-03C 若单独把
`mnemosyne-episodic-compiler/1` 发布为 CURRENT，就会顶掉 Memory OKF 世界。反过来也一样。

因此禁止以下实现：

- 为 Episode 建第二个 CURRENT；
- 扫描 `generations/` 猜“最新 Episodic Generation”；
- CLI 在不同 CURRENT 之间自动切换；
- 把两个独立 Generation 拼成一个伪造的 Retrieval Context。

先完成 MEM-03C-04A Composite Generation：一个事务、一个永久 Manifest、一个 CURRENT、一个
compiled output hash，同时发布 Memory OKF 与 Episodic Card/Index。旧 OKF 与 Episodic Compiler
版本继续注册并永久可重建。Composite Gate 通过后才进入本计划后续 CLI/Profile 实现。

## 二、边界与假设

Reasonix 继续提供 Agent、Subagent、bash 工具、Session 与任务执行；OMR 只提供固定记忆世界、
读取协议和治理。第一版不要求 Reasonix 官方新增接口，也不让 OMR 代理模型调用。

MEM-03C 已有 Episode/Context Fact、编译器、Reader、Receipt 与 Doctor，但尚缺 CLI。Reasonix 不应
直接猜 `.reasonix/omr/memory/**` 私有路径或手写 JSON，因此本阶段只增加只读 CLI 与 Profile
工作流。自动采集真实 Episode 属 MEM-04，不在本阶段偷偷实现。

## 三、CLI 设计

```text
omr memory episodic context --project-dir . [--global-dir ...] --json
omr memory episodic index --context-file <file> --scope project --json
omr memory episodic card --context-file <file> --scope project --episode-id <id> --json
omr memory episodic validate-receipt --context-file <file> --receipt-file <file> --json
omr memory episodic doctor --context-file <file> --scope project --json
```

`context` 是唯一允许解析 CURRENT 的入口：它在一次命令内把当前 Composite Generation 固定成完整
`EpisodicScopeContext`。后续 index/card/receipt/doctor 只接受该固定上下文，不再读取 CURRENT。
上下文文件和回执文件必须走现有项目路径安全、symlink、大小和 strict JSON 校验；stdout 只输出
结构化 JSON，stderr 使用稳定脱敏错误。

如果当前 Generation 不是已注册的 Composite Compiler，`context` 返回 `unavailable`，不把旧
Memory-only 或 Episode-only Generation 冒充为 Composite Generation，也不自动切换或创建 Generation。

## 四、Reasonix Profile 工作流

更新 `assets/skills/omr-memory/SKILL.md`：

1. 父 Agent 遇到需要历史任务经验的工作时，启动一个只读 Librarian Subagent；
2. Librarian 先调用 `context` 固定世界，再逐层读 index/card；
3. Librarian 只返回 EpisodeRef、Card 索引和受限相关性理由；
4. 父 Agent 自己读取推荐 Card，必要时才通过 EvidenceRef 进入后续核验；
5. 无候选返回 `no_candidate`，固定世界不可读返回 `unavailable`，不得猜测；
6. 禁止 Librarian 写代码、修改记忆、切 CURRENT、读取 frozen Memory 或输出完整轨迹；
7. Episode 命中不等于 Memory 被采用，不产生 help/harm。

Profile 仍是 Prompt/Workflow 层，不复制 Reasonix Subagent Runtime。

## 五、自动化端到端

Fake/临时项目测试：

```text
创建规范 Episode/Context
→ 编译并发布 Episodic Generation
→ CLI context
→ CLI index/card
→ Fake Librarian Receipt
→ validate-receipt
→ 发布另一 CURRENT
→ 使用旧 context 重读，字节不变
→ doctor clean
→ 篡改临时派生文件，doctor 报漂移
```

覆盖 CLI 退出码、JSON Schema、路径/symlink/超限、错误脱敏、Project/Global 隔离、无 API Key、
无网络、无真实模型调用。`omr init/upgrade` 后必须安装更新后的 Profile，Manifest Hash 与 Doctor
一致，旧项目升级不覆盖用户 Overlay。

## 六、真实 Reasonix Desktop 联调

自动门禁通过后再需要用户协助：

1. 在临时项目安装当前 OMR；
2. 使用测试脚本发布一份不含敏感数据的 Episodic Generation；
3. 重启 Reasonix Desktop；
4. 让 Reasonix 启动只读 Librarian Subagent，返回一个 Episode Card 索引；
5. 父 Agent 读取 Card 并说明其来源；
6. 切换或发布新 CURRENT，确认旧任务仍读取固定 Generation；
7. 运行 Doctor 并确认无漂移；
8. 删除临时项目。

这只验证“Reasonix 能正确使用 OMR 记忆读取协议”，不宣称模型质量提升。真实自动采集与收益评估
分别属于 MEM-04 和 Benchmark 阶段。

## 七、门禁与交付

先写失败的库级、CLI 进程级和安装联调测试，再最小实现。运行 gofmt、diff check、memory/CLI/
install 测试与 race、全量 test、vet、build、Docs Gate。自动门禁通过后提交推送，但在真实 Desktop
联调前不创建 Tag/Release，也不把 MEM-03C 标为完全验收。
