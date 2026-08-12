# OMR Mnemosyne MEM-03C-03：固定世界 Episode Recall 与 Doctor

- 状态：✅ 已实现（2026-08-12）
- 前置：MEM-03C-02A 规范 Fact、MEM-03C-02B 派生编译器已实现并推送
- 范围：固定 Generation Reader、Recall 协议校验与只读 Doctor；不做语义检索模型、不做采集桥

## 一、关键设计决议

MEM-03B 的 `RetrievalScopeContext` 固定指向 Memory OKF 的 `wiki/index.md` 和
`state/index-tree.json`，不能冒充 Episodic Generation。MEM-03C 新增独立的
`EpisodicScopeContext`，显式携带：

```yaml
scope: project
scope_id: project_...
generation_id: generation_...
input_manifest_id: generation_...
input_manifest_sha256: sha256_...
index_path: state/episodes/index.json
```

它不读取 CURRENT。调用方必须给出完整固定引用；Reader 重新验证 Generation、永久 Manifest、
Compiler Version 与 compiled output hash 后才允许读取 Card/Index。

## 二、Reader API

```go
ReadEpisodicIndex(ctx, store, pinned) (*EpisodicIndex, error)
ReadEpisodeCard(ctx, store, pinned, ref) (*EpisodeCard, error)
```

规则：

- 只读程序生成的固定路径，不接受调用方任意相对路径；
- EpisodeRef 必须在 Index 中恰好出现一次，Card 路径与 Hash 必须吻合；
- 读取路径复用 Generation 完整性与 `readLibrarianFile` 的 O_NOFOLLOW、逐组件 symlink、普通文件、
  0600、UTF-8、大小限制；
- 错误固定脱敏，不返回绝对路径、Evidence ID、命令、摘要或文件内容；
- CURRENT 切换、其他项目 Generation 与 legacy Evolution 目录均不影响固定读取。

## 三、Recall Request / Receipt

Reasonix Librarian 负责自然语言相关性判断；OMR 只验证机器协议：

```yaml
request:
  schema_version: 1
  retrieval_id: retrieval_...
  project: {pinned episodic scope or null}
  global: {pinned episodic scope or null}
  task_summary: "bounded text"
  component_refs: []
  operation_refs: []
  task_class_refs: []
  failure_concept_refs: []

receipt:
  schema_version: 1
  retrieval_id: retrieval_...
  status: found            # found | no_candidate | unknown | unavailable
  episode_cards:
    - episode_ref: {...}
      scope_id: project_...
      card_sha256: sha256_...
      relevance_rank: 1
      why: "bounded reason"
  visited_index_scopes: [project]
  requires_parent_read: true
```

OMR 校验 selected Episode 确实在对应固定 Index 中、Card Hash 正确、rank 从 1 连续且不重复、why
有界且不包含控制字符。`found` 必须非空；其他状态不得夹带 Card。回执不自动读取 Evidence、
不创建 Memory、不产生 help/harm；只有实际读 Card 后，集成层才可记录 `read` 类观察事实。

## 四、Episodic Doctor

```go
CheckEpisodicGeneration(ctx, store, pinned) (*EpisodicDoctorReport, error)
```

只读检查：

1. Generation/Manifest/Compiler/compiled hash；
2. Manifest 中 Episode/Context 输入能否完整解析、Scope/Hash 是否一致；
3. Episode → Context 断链、同 Root Task 多 Episode、未来事实；
4. Index 重复/遗漏/超限/排序漂移、Card 缺失或 Hash 漂移；
5. 从永久 Manifest 重建输出，逐路径逐字节与已发布输出比较；
6. Manifest 是否误收 Card/Index、是否遗漏实际 Episode/Context 输入；
7. legacy Evolution 数据绝不被当作规范输入。

报告稳定排序、固定错误码、无绝对路径和事实正文；不修复、不删除、不切 CURRENT。派生输出被
删除时报告 `rebuildable_missing`，并给出预期 Hash，但不自动落盘。

## 五、TDD 验收矩阵

1. 固定 Generation Index/Card 正常读取；CURRENT 切换后字节不变；
2. Project/Global Scope ID 与 Generation 隔离；
3. 错误 Generation/Manifest/Hash/Compiler、未来引用 fail closed；
4. Card/Index symlink、路径替换、权限、超限、未知字段拒绝；
5. Recall 四状态、rank/去重/why/selected ref 完整矩阵；
6. Receipt 多次验证字节一致，零写入；
7. Doctor clean、缺 Card、篡改 Card、重复/遗漏 Index、断链 Context；
8. 删除全部派生输出后重建 Hash 与原 Generation 一致；
9. legacy Evolution 目录存在时结果不变；
10. 错误与报告不泄露路径、Evidence ID、命令、凭据或内容。

## 六、门禁、提交与下一阶段

执行 gofmt、diff check、memory test/race、全量 test、vet、CLI build、Docs Gate，并做临时项目
Generation 发布→读取→切换 CURRENT→重读→删除派生输出→Doctor 重建验证。全部通过后独立
提交并推送，再进入 MEM-03C-04（Reasonix Profile/Prompt 接入与真实客户端联调）。

本阶段不创建 Tag/Release，不修改 Reasonix 官方仓库。
