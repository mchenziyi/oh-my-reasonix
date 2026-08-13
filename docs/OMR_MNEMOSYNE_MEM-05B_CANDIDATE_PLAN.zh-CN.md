# OMR Mnemosyne MEM-05B：GlobalPromotionCandidate 规范事实

- 阶段：MEM-05B-Candidate
- 状态：✅ 已实现并通过门禁（2026-08-13）；Consistency Doctor 已纳入候选事实完整性检查
- 依据：Architecture v1 §17.5/Global Promotion；现有 `PromotionPlan` 只读对象

## 目标

把跨项目泛化候选从临时计划提升为可审计、不可变的 Global Scope 规范事实。Candidate 不进入普通索引，不代表 Global Active，也不替代最终 Global MemoryRevision。

## Schema

`facts/promotion-candidates/<candidate-id>.json`，绑定 Global FactStore：

- `schema_version`、`candidate_id`、`status`（`collecting|eligible|rejected`）
- `usage_policy`
- `source_memory_refs`（至少两个 Project `MemoryRef`，精确五字段）
- `source_project_family_fingerprints`（去重、稳定排序的 SHA-256）
- `outcome_refs`、`evidence_refs`、`confirmation_source_ref`
- `critic_judgment_refs`
- `proposed_applies_when`、`proposed_does_not_apply_when`
- `content_sha256`

不同 Usage Policy 只允许对应证据字段：`outcome_attributed` 使用 Outcome；`evidence_validated` 使用 Evidence；`explicit_confirmation` 使用 Confirmation。其余证据字段必须为空。Candidate 的 Content Hash 由程序重算，旧事实不可覆盖。

## 写入边界

`Put` 只接受严格 Schema、精确引用和脱敏结构化字段；同 ID 同 Hash 为 NOOP，不同 Hash fail closed。Candidate 仅由显式调用写入，不自动批准、不切换 CURRENT、不修改来源 Project Facts。

## 验收

- Legacy/未知字段/非法策略/跨 Scope/重复 Ref/错误 Hash/策略借用全部拒绝；
- canonical JSON 与排序稳定；
- Global/Project Store 隔离；
- 读写、冲突、幂等、脱敏和权限复用 FactStore；
- `go test -race ./internal/memory/...`、全量门禁和 docs gate 全绿。

## Reasonix Agent 执行提示词

```text
执行 OMR Mnemosyne MEM-05B-Candidate。先读取本计划、Architecture v1 Global Promotion 章节、MEM-01A~F、MEM-02 与现有 PromotionPlan/FactStore。
先写失败测试，再实现最小 GlobalPromotionCandidate Fact：扩展 FactKind 与 decode/factKey/factScope 路由，严格 Validate/canonMap/ContentHash/EncodeCanonical，复用现有 Store 的权限、symlink、NOOP、冲突、脱敏和 scope 隔离。
Candidate 必须至少包含两个 Project MemoryRef、去重排序的 family fingerprint，并按 UsagePolicy 限制 outcome/evidence/confirmation 字段；不新增自由文本、路径、Prompt、命令或模型字段。不要修改 Architecture v1，不自动批准，不切换 CURRENT，不实现 Global Active。
先运行针对性 memory 测试，再运行 gofmt、git diff --check、go test -race ./internal/memory/...、go test ./...、go vet ./...、go build ./cmd/omr、bash tests/docs_check.sh；做 review/security review 后再报告，不要自行提交或推送。
```
