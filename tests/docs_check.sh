#!/usr/bin/env bash
# docs_check.sh — LP-06 documentation consistency gate.
# Checks, in order:
#   1. No stale version-status phrases ("v2.0.0 进行中", "T14 待实现", etc).
#   2. Every relative Markdown link in README.md / README.en.md resolves.
#   3. Code-fenced command examples contain no machine-specific paths
#      (/Users, /home/, C:\), and parse as expected tool invocations.
#   4. The "current available capability" matrix exists and cites real files.
#   5. README.zh / README.en core status agree on the delivered version.
set -euo pipefail

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

fail() {
  echo "docs check FAILED: $*" >&2
  exit 1
}

# --- 1. stale phrases -------------------------------------------------------
stale=(
  'v2\.0\.0 进行中'
  'v2\.0\.1 进行中'
  'T14 待实现'
  'T13 待实现'
  '尚未实现.*Comment Checker'
  'LP-0[1-6].*进行中'
  'Hook.*待开发'
)
for pat in "${stale[@]}"; do
  # Scan active docs plus the T13/T14/POST-V2 task books (which are archived
  # but may drift); historical execution reports are excluded.
  if grep -rEn "$pat" README.md README.en.md \
      docs/OMR_TODO_LATEST.zh-CN.md docs/OMR_VS_OMO_GAP_MATRIX.zh-CN.md \
      docs/OMR_CAPABILITY_MATRIX.zh-CN.md \
      docs/OMR_T13_COMMENT_CHECKER_AND_DOCS_PLAN.zh-CN.md \
      docs/OMR_T14_COMMENT_CHECKER_RUNTIME_HOOK_PLAN.zh-CN.md \
      docs/OMR_POST_V2_LOCAL_ENHANCEMENT_PLAN.zh-CN.md 2>/dev/null | grep -v 'Archived\|已归档\|历史\|不再出现'; then
    fail "stale phrase '$pat' found"
  fi
done

# --- 1b. Mnemosyne specification convergence -------------------------------
mnemosyne="docs/OMR_EVOLUTION_MEMORY_OKF_ARCHITECTURE.zh-CN.md"
[ -f "$mnemosyne" ] || fail "missing $mnemosyne"

grep -q '状态：正式实现规格' "$mnemosyne" || fail "Mnemosyne spec is not formally frozen"

mnemosyne_required=(
  'project_generation'
  'global_generation'
  'MemoryRef'
  'EvidenceRef'
  'JudgmentRef'
  'ConfirmationSourceRef'
  'usage_policy'
  'context_signature_version'
  'context_descriptor_ref'
  'facts/memory-revisions/'
  'facts/memory-evidence-generations/'
  'facts/governance-events/'
  'facts/retrieval-evaluations/'
  'facts/judgments/'
  'facts/policies/'
  'Judgment Fact 未知字段拒绝、永久不可变'
  '撤销确认或修正 Override 必须创建新 Judgment'
  'facts/generation-input-manifests/'
  'facts/memory-mutations/'
  'GlobalPromotionCandidate'
  'Global Promotion × Usage Policy'
  'OKF Page = MemoryRevision + 当前 MemoryEvidenceGeneration + DerivedMemoryState'
  'CURRENT.*切换前，构建目标 Generation 所需的全部规范事实必须已经安全落盘'
  'prepared manifest'
  'input_manifest_sha256'
  'memory_compiler_version_unavailable'
  'Generation Input Manifest 必须在 staging 验证完成后、`CURRENT` 切换前安全落盘'
  'child specializes parent'
  '任何 `usage_policy=explicit_confirmation` 的 MemoryRevision 都必须携带合法 `confirmation_source_ref`'
  'Merge 主 ID 只按证据链完整度、创建时间和 memory_id 确定性选择'
  '任何派生状态都必须能够从规范事实源确定性重建'
  'Architecture v1 — Frozen'
  'RetrievalEvaluation'
  'retrieval_relevance'
  'Episodic Recall'
  'Episode Card 与 Episodic Index 是 Generation 内的派生读取表示'
  'context_applicability'
  'ApplicabilityCondition'
  'Evidence Provenance 与 Trust Judgment'
  'ContentClassificationRef'
  'PolicyRef'
  'freshness_evaluation'
  'observation_provenance'
  '确定性 Fan-out 与自动分片'
  'Memory Quality Benchmark'
  'benchmark_id: mnemosyne_quality_v1'
  'Implementation Failure'
  'Benchmark Failure'
)
for required in "${mnemosyne_required[@]}"; do
  grep -q "$required" "$mnemosyne" || fail "Mnemosyne spec missing required contract '$required'"
done

# Legacy forms may be named in the gate/migration prose, but must not appear
# as active commands, paths, or Schema fields.
mnemosyne_forbidden_active=(
  '^[[:space:]]*omr evolve memory'
  '^[[:space:]]*\.reasonix/omr/evolution/wiki/'
  '^[[:space:]]*not_for:'
  '^[[:space:]]*failure_classes:'
  '^[[:space:]]*confidence:'
  '^[[:space:]]*(success_count|failure_count|helped_count|harmed_count):'
  '^[[:space:]]*"(success_count|failure_count|helped_count|harmed_count)"[[:space:]]*:'
  '原样恢复：`frozen → probation`'
  '查看 confirmed help/harm'
  '三次可归因失败后冻结'
  '成功采用次数更多'
  '^[[:space:]]*freshness:[[:space:]]*invalidated'
  '^[[:space:]]*source_context_ref:'
  '^[[:space:]]*instructional_content_allowed:[[:space:]]*true'
)
for pat in "${mnemosyne_forbidden_active[@]}"; do
  if grep -En "$pat" "$mnemosyne"; then
    fail "Mnemosyne spec contains active legacy form '$pat'"
  fi
done

# --- 1d. Mnemosyne MEM-03A index sharding plan ----------------------------
index_plan="docs/OMR_MNEMOSYNE_MEM-03A_INDEX_SHARDING_PLAN.zh-CN.md"
[ -f "$index_plan" ] || fail "missing $index_plan"

index_required=(
  '状态：✅ 已实现并通过门禁'
  'Index 只能是 Generation 派生视图'
  'IndexPolicyRef PolicyRef'
  'Policy Fact 作为 `ManifestInput`'
  '最终规范 UTF-8 渲染结果'
  'component → operation → memory_type → stable_id_prefix'
  'Root 超限后只保存路由摘要'
  '禁止截断、丢弃、随机'
  'memory_index_policy_unsatisfied'
  '不实现 Librarian、Episode Index、CLI、Prompt 或模型调用'
  '交给 Reasonix 的完整执行提示词'
)
for required in "${index_required[@]}"; do
  grep -q "$required" "$index_plan" || fail "MEM-03A plan missing contract '$required'"
done

if grep -Eq '新增 (facts/index|FactKindIndex)|引入向量数据库|允许静默使用默认|自动使用 defaultIndexPolicy' "$index_plan"; then
  fail "MEM-03A plan contains a forbidden index architecture"
fi

# --- 1e. Mnemosyne MEM-03B Librarian plan ---------------------------------
librarian_plan="docs/OMR_MNEMOSYNE_MEM-03B_LIBRARIAN_PLAN.zh-CN.md"
[ -f "$librarian_plan" ] || fail "missing $librarian_plan"

librarian_required=(
  '状态：✅ 已实现并通过门禁'
  '固定 Project / Global Generation Pair'
  '程序不分析自然语言语义'
  'requires_parent_read'
  '正常模式必须严格为空'
  '不设置 Librarian 总页面数'
  'Profile `omr-memory`'
  '不实现：`omr memory context` CLI'
  '交给 Reasonix 的完整执行提示词'
  '十一、实现结果'
)
for required in "${librarian_required[@]}"; do
  grep -q "$required" "$librarian_plan" || fail "MEM-03B plan missing contract '$required'"
done

if grep -Eq '允许引入向量数据库|允许使用 Embedding|允许自动切换 CURRENT|正常模式允许 include-frozen|允许 Librarian 写项目' "$librarian_plan"; then
  fail "MEM-03B plan contains a forbidden Librarian architecture"
fi

# --- 1f. Mnemosyne MEM-03C Episodic Recall plan --------------------------
episodic_plan="docs/OMR_MNEMOSYNE_MEM-03C_EPISODIC_RECALL_PLAN.zh-CN.md"
[ -f "$episodic_plan" ] || fail "missing $episodic_plan"

episodic_required=(
  '状态：✅ MEM-03C-01～04 自动化实现完成，待真实 Reasonix Desktop 联调'
  'MEM-03C-01 Schema Gate'
  'internal/evolution.Episode'
  'Episode Card / Episodic Index'
  '不能直接成为'
  '不读取 CURRENT'
  '只允许记录 `retrieved/read`'
  '交给 Reasonix 的第一阶段执行提示词'
)
for required in "${episodic_required[@]}"; do
  grep -q "$required" "$episodic_plan" || fail "MEM-03C plan missing contract '$required'"
done

episodic_compiler_plan="docs/OMR_MNEMOSYNE_MEM-03C_02B_COMPILER_PLAN.zh-CN.md"
[ -f "$episodic_compiler_plan" ] || fail "missing $episodic_compiler_plan"
for required in '状态：✅ 已实现' '不读取 CURRENT' 'Evolution Store' '每个 Episode 恰好出现一次'; do
  grep -q "$required" "$episodic_compiler_plan" || fail "MEM-03C compiler plan missing '$required'"
done

episodic_recall_doctor_plan="docs/OMR_MNEMOSYNE_MEM-03C_03_RECALL_DOCTOR_PLAN.zh-CN.md"
[ -f "$episodic_recall_doctor_plan" ] || fail "missing $episodic_recall_doctor_plan"
for required in '状态：✅ 已实现' '不能冒充 Episodic Generation' '它不读取 CURRENT' '不自动读取 Evidence' '不修复、不删除、不切 CURRENT'; do
  grep -q "$required" "$episodic_recall_doctor_plan" || fail "MEM-03C Recall/Doctor plan missing '$required'"
done

episodic_reasonix_plan="docs/OMR_MNEMOSYNE_MEM-03C_04_REASONIX_INTEGRATION_PLAN.zh-CN.md"
[ -f "$episodic_reasonix_plan" ] || fail "missing $episodic_reasonix_plan"
for required in '状态：✅ 自动化实现完成，待真实 Reasonix Desktop 联调' '不要求 Reasonix 官方新增接口' 'context` 是唯一允许解析 CURRENT 的入口' '启动一个只读 Librarian Subagent' '自动采集真实 Episode 属 MEM-04'; do
  grep -q "$required" "$episodic_reasonix_plan" || fail "MEM-03C Reasonix integration plan missing '$required'"
done

composite_plan="docs/OMR_MNEMOSYNE_MEM-03C_04A_COMPOSITE_GENERATION_PLAN.zh-CN.md"
[ -f "$composite_plan" ] || fail "missing $composite_plan"
for required in '状态：✅ Schema Gate 与 Composite Compiler 已实现' '共享唯一 `CURRENT`' 'mnemosyne-composite-compiler/1' '一个 Manifest + 一个 compiled_output_sha256 + 一个 CURRENT' '不实现 `omr memory episodic context`'; do
  grep -q "$required" "$composite_plan" || fail "MEM-03C Composite plan missing '$required'"
done

usage_capture_plan="docs/OMR_MNEMOSYNE_MEM-04A_USAGE_CAPTURE_PLAN.zh-CN.md"
[ -f "$usage_capture_plan" ] || fail "missing $usage_capture_plan"
for required in '状态：✅ 自动化实现完成，待真实 Reasonix Desktop 回执联调' '回执不是新 Fact' '每个任务只记录最终阶段' '禁止 `time.Now()`' 'Episode EvidenceRef 固定为 `evidence_type=episode`' '未产生 Outcome 前 help/harm 均为 0' '知识型 LibrarianReceipt 的' '没有实现' 'Outcome、归因'; do
  grep -q "$required" "$usage_capture_plan" || fail "MEM-04A Usage Capture plan missing '$required'"
done

outcome_plan="docs/OMR_MNEMOSYNE_MEM-04B_ATTRIBUTION_OUTCOME_PLAN.zh-CN.md"
[ -f "$outcome_plan" ] || fail "missing $outcome_plan"
for required in '状态：✅ 自动化实现完成，待真实 Reasonix Desktop Attribution 回执联调' 'AttributionReceipt 是瞬时协议' 'Outcome 采用 Legacy / Enriched 双形态' '模型不得提供 Outcome ID' '同一 `root_task_id + MemoryRef + context_signature`' '第三方失败无法触发冻结' '不读取墙钟' '当前没有自动调用 Attribution Analyst/Critic' '不得进入 MEM-04C'; do
  grep -q "$required" "$outcome_plan" || fail "MEM-04B plan missing contract '$required'"
done

governance_plan="docs/OMR_MNEMOSYNE_MEM-04C_LIFECYCLE_GOVERNANCE_PLAN.zh-CN.md"
[ -f "$governance_plan" ] || fail "missing $governance_plan"
for required in '状态：✅ 自动化实现完成' '事件严格作用于目标 Revision' '自动冻结与 manual_freeze 分离' 'Unfreeze 硬门禁' 'Governance Event 本身不能解除自动冻结证据' '只有同时声明 `--review-mode`' '旧 Revision 的 Governance Event 不污染新 Revision' '不进入 MEM-04D'; do
  grep -q "$required" "$governance_plan" || fail "MEM-04C plan missing contract '$required'"
done
if grep -Eq '模型直接提供 `(usage_id|content_sha256)`|自动生成 `Outcome`|自动判断 helped/harmed|读取最新 CURRENT' "$usage_capture_plan"; then
  fail "MEM-04A plan contains a forbidden Usage Capture architecture"
fi

if grep -Eq '将 Episode Card 新增为 FactKind|允许引入向量数据库|允许使用 Embedding|直接复用 internal/evolution.Episode|允许模型提供可信 Hash' "$episodic_plan"; then
  fail "MEM-03C plan contains a forbidden Episodic Recall architecture"
fi

episodic_audit="docs/OMR_MNEMOSYNE_MEM-03C_SCHEMA_GATE_AUDIT.zh-CN.md"
[ -f "$episodic_audit" ] || fail "missing $episodic_audit"
for required in 'Schema Gate：PASS' '一个 Root Task 聚合为一个 Episode' 'task_result' 'EpisodeFact v1 不保存自由文本摘要' 'canonical_sha256 与 content_sha256 分离' 'Card/Index 不进入 inputs'; do
  grep -q "$required" "$episodic_audit" || fail "MEM-03C Schema Gate audit missing '$required'"
done

mutation_plan=$(sed -n '/^## 6\.8 MemoryMutationPlan/,/^## 6\.9 /p' "$mnemosyne")
if printf '%s\n' "$mutation_plan" | grep -Eq 'before_content_sha256|after_content_sha256|content_sha256:[[:space:]]*sha256_(old|new)'; then
  fail "Mnemosyne MemoryMutationPlan lets the model supply trusted before/after hashes"
fi

line_10_1=$(grep -n '^### 10\.1 ' "$mnemosyne" | cut -d: -f1)
line_10_2=$(grep -n '^### 10\.2 ' "$mnemosyne" | cut -d: -f1)
line_10_3=$(grep -n '^### 10\.3 ' "$mnemosyne" | cut -d: -f1)
if [ -z "$line_10_1" ] || [ -z "$line_10_2" ] || [ -z "$line_10_3" ] || \
   [ "$line_10_1" -ge "$line_10_2" ] || [ "$line_10_2" -ge "$line_10_3" ]; then
  fail "Mnemosyne section 10 order is inconsistent"
fi

# --- 1c. Mnemosyne Protocol Extension convergence --------------------------
# The Protocol Extension design (the whole document: chapters 1-9 normative
# Schema plus chapter 11 final decisions) must stay aligned with Architecture
# v1 and the MEM-02 Schema Convergence decisions (D1-D12). The initial Schema
# Gate audit lives in a separate archived file that is NOT protocol input, so
# only the main protocol document is scanned for forbidden terms.
pe_doc="docs/OMR_MNEMOSYNE_MEM-02_PROTOCOL_EXTENSION_PLAN.zh-CN.md"
[ -f "$pe_doc" ] || fail "missing $pe_doc"
pe_body=$(cat "$pe_doc")

pe_required=(
  'schema_version: 1'
  'judgment_type: retrieval_relevance'
  'subject_type: retrieval'
  'retrieval_id: retrieval_'
  'memory_context:'
  'project_generation_ref: ProjectGenerationRef | null'
  'global_generation_ref: GlobalGenerationRef | null'
  'judgment_type: critic_review'
  'passed | failed | unavailable'
  'fixture_oracle | offline_rule | user_review'
  'runtime | user | official | project | external'
  'direct | tool_observed | model_extracted | imported'
  'verified | confirmed | inferred | unverified'
  'provenance_refs: [EvidenceRef]'
  'basis_context_refs'
  'required_condition_ids'
  'result: exact | applicable | conditionally_applicable | not_applicable | unknown'
  'observation_provenance'
  'Schema Gate：PASS'
  'SCHEMA_GATE_AUDIT'
)
for required in "${pe_required[@]}"; do
  printf '%s\n' "$pe_body" | grep -Fq "$required" || fail "Protocol Extension design missing required contract '$required'"
done

pe_forbidden=(
  'schema_version: 2'
  'EvidenceTrustFact'
  'candidate_universe'
  'CandidateUniverseRef'
  'authority:'
  'trust_policy_sha256'
  'source_context_ref'
  'retrieval_generation_ref'
  'evaluation_generation_ref'
  'ProvenanceRef'
  '[ContextRef]'
  'Gate 未通过'
  '## 十、Schema Gate 审核结果'
)
for pat in "${pe_forbidden[@]}"; do
  if printf '%s\n' "$pe_body" | grep -Fq "$pat"; then
    fail "Protocol Extension design contains forbidden form '$pat'"
  fi
done

# The initial Schema Gate audit is archived history, not protocol input.
pe_audit="docs/OMR_MNEMOSYNE_MEM-02_SCHEMA_GATE_AUDIT.zh-CN.md"
[ -f "$pe_audit" ] || fail "missing archived Schema Gate audit $pe_audit"
grep -q '历史归档' "$pe_audit" || fail "Schema Gate audit must be marked as archived history"
grep -q '不作为当前协议输入' "$pe_audit" || fail "Schema Gate audit must state it is not protocol input"

# --- 1d. Mnemosyne MEM-02C delivery status and Legacy boundary ------------
mem02_doc="docs/OMR_MNEMOSYNE_MEM-02_PLAN.zh-CN.md"
mem02c_doc="docs/OMR_MNEMOSYNE_MEM-02C_EVIDENCE_TRUST_PLAN.zh-CN.md"
[ -f "$mem02_doc" ] || fail "missing $mem02_doc"
[ -f "$mem02c_doc" ] || fail "missing $mem02c_doc"
grep -Fq 'MEM-02C Evidence Provenance + Trust Gate 已完成并签收' "$mem02_doc" || fail "MEM-02 plan does not mark MEM-02C signed off"
if grep -Fq 'MEM-02C 待实现' "$mem02_doc"; then
  fail "MEM-02 plan still marks MEM-02C pending implementation"
fi
grep -Fq 'Legacy 完成这些验证后受控短路为 `unavailable`' "$mem02c_doc" || fail "MEM-02C plan missing the Legacy controlled short-circuit rule"
grep -Fq 'Generation 路径与正文身份' "$mem02c_doc" || fail "MEM-02C plan missing exact Generation identity validation"

# --- 1e. Mnemosyne MEM-02D implementation boundary ------------------------
mem02d_doc="docs/OMR_MNEMOSYNE_MEM-02D_RETRIEVAL_EVALUATION_PLAN.zh-CN.md"
[ -f "$mem02d_doc" ] || fail "missing $mem02d_doc"
grep -Fq '状态：✅ 已实现' "$mem02d_doc" || fail "MEM-02D plan is not marked implemented"
grep -Fq 'RetrievalEvaluation Fact + retrieval_relevance Judgment 双对象' "$mem02d_doc" || fail "MEM-02D plan missing the dual-object contract"
grep -Fq '固定 Project/Global Generation Pair' "$mem02d_doc" || fail "MEM-02D plan missing the fixed-world contract"
grep -Fq '不读 CURRENT' "$mem02d_doc" || fail "MEM-02D plan missing the no-CURRENT boundary"
grep -Fq '未进入 MEM-02E' "$mem02d_doc" || fail "MEM-02D plan missing the phase boundary"

# --- 1f. Mnemosyne MEM-02E frozen implementation contract -----------------
mem02e_doc="docs/OMR_MNEMOSYNE_MEM-02E_CONTEXT_APPLICABILITY_PLAN.zh-CN.md"
[ -f "$mem02e_doc" ] || fail "missing $mem02e_doc"
grep -Fq '状态：✅ 已实现' "$mem02e_doc" || fail "MEM-02E plan is not marked implemented"
grep -Fq 'BasisContextRefs []string `json:"basis_context_refs,omitempty"`' "$mem02e_doc" || fail "MEM-02E plan missing the top-level basis context field"
grep -Fq 'Legacy / Enriched 双形态' "$mem02e_doc" || fail "MEM-02E plan missing the Legacy compatibility boundary"
grep -Fq 'exact | applicable | conditionally_applicable | not_applicable | unknown' "$mem02e_doc" || fail "MEM-02E plan missing the frozen result enum"
grep -Fq '不把 `unavailable` 写进 Judgment' "$mem02e_doc" || fail "MEM-02E plan confuses derived unavailable with persisted result"
grep -Fq 'Context Descriptor Fact 已实现' "$mem02e_doc" || fail "MEM-02E plan is missing the Context Descriptor implementation status"
grep -Fq '## 九、交给 Reasonix 的完整执行提示词' "$mem02e_doc" || fail "MEM-02E plan missing the Reasonix execution prompt"
grep -Fq '未进入 MEM-02F、未提交、未推送' "$mem02e_doc" || fail "MEM-02E plan missing the phase boundary"

if grep -Fq 'exact` 并入 `applicable`' "$mem02_doc" || \
   grep -Fq 'not_applicable | unknown | unavailable`' "$mem02_doc"; then
  fail "MEM-02 plan contains the superseded Context Applicability result proposal"
fi

# --- 1g. Mnemosyne MEM-02F frozen implementation contract -----------------
mem02f_doc="docs/OMR_MNEMOSYNE_MEM-02F_FRESHNESS_REVALIDATION_PLAN.zh-CN.md"
[ -f "$mem02f_doc" ] || fail "missing $mem02f_doc"
grep -Fq '状态：✅ 已实现并签收' "$mem02f_doc" || fail "MEM-02F implementation status is stale"
grep -Fq '不新增 Schema 字段' "$mem02f_doc" || fail "MEM-02F plan attempts an unapproved Schema expansion"
grep -Fq 'PolicyRef.content_sha256' "$mem02f_doc" || fail "MEM-02F plan missing the unique Policy hash anchor"
grep -Fq 'fresh | aging | needs_revalidation' "$mem02f_doc" || fail "MEM-02F plan missing the frozen Freshness enum"
grep -Fq 'evaluation_expired' "$mem02f_doc" || fail "MEM-02F plan missing judgment expiry semantics"
grep -Fq 'revalidation_evidence_types' "$mem02f_doc" || fail "MEM-02F plan missing evidence type filtering"
grep -Fq '## 八、交给 Reasonix 的完整执行提示词' "$mem02f_doc" || fail "MEM-02F plan missing the Reasonix execution prompt"
grep -Fq '未进入 MEM-02G、未提交、未推送' "$mem02f_doc" || fail "MEM-02F plan missing the phase boundary"
grep -Fq 'conflicting_freshness_judgments' "$mem02f_doc" || fail "MEM-02F plan missing implemented conflict fallback"
grep -Fq 'stale_window' "$mem02f_doc" || fail "MEM-02F plan missing implemented stale-window reason"

if grep -Fq 'payload 增加 `freshness_policy_sha256`' "$mem02_doc" || \
   grep -Fq 'content_classification_ref`（受约束 JudgmentRef' "$mem02_doc"; then
  fail "MEM-02 plan contains the superseded Freshness Schema proposal"
fi

# --- 1h. Mnemosyne MEM-02G Conflict Review Schema Gate --------------------
mem02g_doc="docs/OMR_MNEMOSYNE_MEM-02G_CONFLICT_REVIEW_PLAN.zh-CN.md"
[ -f "$mem02g_doc" ] || fail "missing $mem02g_doc"
grep -Fq '状态：✅ 已完成并经 CTO 签收（2026-08-12）' "$mem02g_doc" || fail "MEM-02G implementation status is stale"
grep -Fq '不新增独立 Conflict FactKind' "$mem02g_doc" || fail "MEM-02G must not create a second conflict fact source"
grep -Fq 'judgment_type: conflict_review' "$mem02g_doc" || fail "MEM-02G missing conflict_review schema"
grep -Fq 'clear | conflict | unavailable' "$mem02g_doc" || fail "MEM-02G missing frozen result vocabulary"
grep -Fq 'sampled_audit + clear' "$mem02g_doc" || fail "MEM-02G missing sampled-clear restriction"
grep -Fq '独立 EvidenceRef >= 3' "$mem02g_doc" || fail "MEM-02G missing evidence gate threshold"
grep -Fq 'CriticRequirement == passed' "$mem02g_doc" || fail "MEM-02G missing critic gate"
grep -Fq 'ConflictRequirement == clear' "$mem02g_doc" || fail "MEM-02G missing conflict gate"
grep -Fq '## 九、交给 Reasonix 的完整执行提示词' "$mem02g_doc" || fail "MEM-02G missing Reasonix prompt"
grep -Fq '未进入 MEM-03' "$mem02g_doc" || fail "MEM-02G phase boundary is missing"

# --- 2. link check (README pair) -------------------------------------------
check_links() {
  local file="$1"
  local dir
  dir=$(dirname "$file")
  # Relative markdown links: [..](path) and [..](path#anchor)
  grep -oE '\]\([^)]+\)' "$file" | sed -E 's/^\]\(//; s/\)$//' | while read -r link; do
    case "$link" in
      http://*|https://*|mailto:*|\#*) continue ;;
      *) ;;
    esac
    target="${link%%#*}"
    if [ -z "$target" ]; then continue; fi
    if [ ! -e "$dir/$target" ]; then
      echo "docs check: broken link '$link' in $file" >&2
      exit 1
    fi
  done
}
check_links README.md || fail "broken link in README.md"
check_links README.en.md || fail "broken link in README.en.md"

# --- 3. command examples ----------------------------------------------------
# Examples must not embed machine-specific paths.
if grep -rn '/Users/\|/home/\|C:\\Users' README.md README.en.md | grep -v '说明\|example\|例如'; then
  fail "machine-specific path in command examples"
fi

# --- 4. capability matrix ---------------------------------------------------
matrix="docs/OMR_CAPABILITY_MATRIX.zh-CN.md"
[ -f "$matrix" ] || fail "missing $matrix"
grep -q 'CLI' "$matrix" || fail "matrix missing CLI scope"
grep -q 'Desktop' "$matrix" || fail "matrix missing Desktop scope"
grep -q 'Reasonix 官方' "$matrix" || fail "matrix missing official-interface scope"

# --- 4b. no contradictory archive banners ------------------------------------
# Host-interface docs must carry exactly one BLOCKED banner and never a
# "已完成交付" banner alongside it.
for f in docs/REASONIX_TASK_MONITOR_DEVELOPMENT_PLAN.zh-CN.md \
         docs/REASONIX_TASK_MONITOR_TM04_PLAN.zh-CN.md \
         docs/REASONIX_TASK_MONITOR_TM05_PLAN.zh-CN.md \
         docs/REASONIX_INTEGRATION_PLAN.zh-CN.md; do
  [ -f "$f" ] || fail "missing host-interface doc $f"
  if grep -q '已完成交付' "$f" && grep -q 'BLOCKED' "$f"; then
    fail "contradictory archive banners in $f"
  fi
done

# --- 5. zh/en status agreement ---------------------------------------------
# Compare the highest v2.0.x version each README mentions (POSIX-safe).
max_ver() {
  # Extract all v2.0.x matches and keep the numerically largest (POSIX-safe).
  grep -oE 'v2\.0\.[0-9]+' "$1" | awk -F. '{ v = $3 + 0; if (v > max) max = v } END { if (max != "") print "v2.0." max }'
}
zh_ver=$(max_ver README.md)
en_ver=$(max_ver README.en.md)
if [ "$zh_ver" != "$en_ver" ]; then
  fail "README zh/en version mismatch: zh='$zh_ver' en='$en_ver'"
fi

echo "docs check: PASS"
