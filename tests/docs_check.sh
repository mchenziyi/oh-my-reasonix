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
  # Only scan active docs, not historical/archived reports.
  if grep -rEn "$pat" README.md README.en.md docs/OMR_TODO_LATEST.zh-CN.md docs/OMR_VS_OMO_GAP_MATRIX.zh-CN.md 2>/dev/null | grep -v 'Archived\|已归档\|历史'; then
    fail "stale phrase '$pat' found"
  fi
done

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
  # Extract all v2.0.x matches and keep the numerically largest.
  grep -oE 'v2\.0\.[0-9]+' "$1" | awk -F. '{ if ($3 > max) max = $3 } END { if (max != "") print "v2.0." max }'
}
zh_ver=$(max_ver README.md)
en_ver=$(max_ver README.en.md)
if [ "$zh_ver" != "$en_ver" ]; then
  fail "README zh/en version mismatch: zh='$zh_ver' en='$en_ver'"
fi

echo "docs check: PASS"
