# OMR Mnemosyne MEM-03C-04A：Composite Generation Schema Gate 与开发计划

- 状态：🟡 Schema Gate 待执行
- 原因：Memory OKF 与 Episodic Recall 共享唯一 `CURRENT`
- 目标：一个不可变 Generation 同时发布两类派生读取表示

## 一、唯一可接受模型

```text
MemoryRevision / Evidence / Policy / Judgment / Usage / Outcome
EpisodeFact / ContextDescriptorFact
                 ↓
mnemosyne-composite-compiler/1
                 ↓
同一个 Generation
├── wiki/index.md
├── wiki/<memory pages...>
├── state/index-tree.json
├── state/memories.json
├── wiki/episodes/index.md
├── wiki/episodes/cards/*.md
├── state/episodes/index.json
└── state/episodes/cards/*.json
                 ↓
一个 Manifest + 一个 compiled_output_sha256 + 一个 CURRENT
```

Composite Compiler 只组合已经冻结的 `CompileOKF` 与 `CompileEpisodic`，不新增事实源：

1. 显式接受两套引用和同一个 Scope/EvaluationTime；
2. 分别调用两个纯编译器；
3. 合并输出时任何路径碰撞 fail closed；
4. Manifest inputs 合并后按 `fact_type + fact_id` 去重，异 Hash 冲突；
5. 对合并后的完整输出集计算唯一 compiled hash；
6. Generation 事务顺序、CAS、崩溃恢复与 CURRENT 语义完全复用 MEM-01D；
7. 不扫描最新事实、不读 Evolution Store、不调用模型或网络。

## 二、版本兼容

- 新注册 `mnemosyne-composite-compiler/1` + canonicalization version 1；
- `mnemosyne-okf-compiler/1|2` 与 `mnemosyne-episodic-compiler/1` 保持注册，历史 Generation 永久可读；
- 新生产路径只发布 Composite；旧 Generation 不迁移、不重写；
- Reader 必须按 Generation 记录的 compiler version 选择能力：旧 OKF 没有 Episode，旧 Episodic
  没有 Memory，均返回明确 `unavailable`，不猜测缺失输出。

## 三、Root Index 与路由

`wiki/index.md` 仍是模型入口。Composite 编译时在 OKF Root Index 增加一个确定性 Episodic 路由，
指向 `wiki/episodes/index.md`；机器 Memory 路由仍由 `state/index-tree.json` 管理，Episodic 机器路由
仍由 `state/episodes/index.json` 管理，二者不互相冒充。

Root Index 的 Episodic 路由属于派生展示，不进入 Fact/Manifest。旧 OKF golden 不能变化；只有新
Composite Compiler 生成带 Episodic 路由的 Root Index。

## 四、TDD 验收矩阵

1. Memory-only、Episode-only、二者都有、二者都空的确定性输出；
2. 输入乱序/重复后输出与 Manifest bytes 稳定；
3. 输出路径碰撞、Fact 身份冲突、跨 Scope、未来输入 fail closed；
4. 合并输出 Hash 与 Generation 发布后 Hash 一致；
5. CURRENT 只切换一次，发布后两类 Reader 同时可用；
6. CAS 冲突保留孤儿 Composite Generation/Manifest，不产生双 CURRENT；
7. 删除输出后由永久 Manifest 重建逐字节一致；
8. 旧 OKF/Episodic Compiler registry 与历史测试不回归；
9. Root Index 只在 Composite 版本增加 Episodic 路由；
10. symlink、权限、大小、敏感信息和错误脱敏继承既有安全链。

## 五、阶段顺序

```text
04A-01 只读 Schema Gate：核对单 CURRENT、Manifest、Compiler Registry、Root Index
04A-02 失败测试 + Composite 纯编译器
04A-03 Generation 事务发布、重建、CAS 与双 Reader 联调
04A-04 Docs Gate 与完整门禁
04B    继续 Reasonix CLI/Profile 接入
```

04A Gate 未 PASS 前，不实现 `omr memory episodic context`，不发布新的 CURRENT，不修改 Profile。
