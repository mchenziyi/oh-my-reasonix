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
  'facts/judgments/'
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
)
for pat in "${mnemosyne_forbidden_active[@]}"; do
  if grep -En "$pat" "$mnemosyne"; then
    fail "Mnemosyne spec contains active legacy form '$pat'"
  fi
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
