#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
project_dir=$(mktemp -d "${TMPDIR:-/tmp}/omr-cli-smoke.XXXXXX")
fake_binary="$project_dir/fake-reasonix"
capture="$project_dir/session-args.txt"
export OMR_SMOKE_MODEL="deepseek-v4-flash"
trap 'rm -rf "$project_dir"' EXIT

printf '[agent]\n' > "$project_dir/reasonix.toml"
mkdir -p "$project_dir/prompts"
printf 'research prompt\n' > "$project_dir/prompts/research.md"
cat > "$fake_binary" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$OMR_SESSION_CAPTURE"
EOF
chmod +x "$fake_binary"

cd "$repo_dir"
go run ./cmd/omr init --project-dir "$project_dir" >/dev/null
mkdir -p "$project_dir/.reasonix/omr"
mkdir -p "$project_dir/.reasonix/omr"
cat > "$project_dir/.reasonix/omr/config.toml" <<'EOF'
[agent.omr-research]
model = "$OMR_SMOKE_MODEL" // smoke model
prompt_file = "prompts/research.md" // existing prompt
read_only = true // read-only profile
EOF
go run ./cmd/omr doctor --project-dir "$project_dir" --json > "$project_dir/doctor.json"
go run ./cmd/omr profile list --project-dir "$project_dir" --json > "$project_dir/profiles.json"
go run ./cmd/omr config validate --project-dir "$project_dir" --config "$project_dir/.reasonix/omr/config.toml" --json > "$project_dir/config.json"
go run ./cmd/omr config schema > "$project_dir/schema.json"
go run ./cmd/omr benchmark quality --replay --fixtures benchmarks/fixtures/metrics-summary --max-cost 1 --output "$project_dir/quality-pass.json"
go run ./cmd/omr upgrade --project-dir "$project_dir" --dry-run > "$project_dir/upgrade.txt"
go run ./cmd/omr uninstall --project-dir "$project_dir" --dry-run > "$project_dir/uninstall.txt"

grep -q '"name":"manifest"' "$project_dir/doctor.json"
grep -q '"id":"omr-explore"' "$project_dir/profiles.json"
grep -q '"id":"omr-research"' "$project_dir/profiles.json"
grep -q '"id":"omr-debug"' "$project_dir/profiles.json"
grep -q '"model":"deepseek-v4-flash"' "$project_dir/profiles.json"
grep -q '"valid":true' "$project_dir/config.json"
grep -q '"agent"' "$project_dir/schema.json"
grep -q '"additionalProperties": false' "$project_dir/schema.json"
grep -q '"qualified_rate": 1' "$project_dir/quality-pass.json"
if go run ./cmd/omr benchmark quality --replay --fixtures benchmarks/fixtures/metrics-summary --max-cost 0.1 > "$project_dir/quality-fail.txt" 2>&1; then
  echo "expected quality max-cost gate to fail" >&2
  exit 1
fi
grep -q 'cost' "$project_dir/quality-fail.txt"
grep -q 'NOOP\|PLAN' "$project_dir/upgrade.txt"
grep -q 'PLAN\|REMOVE' "$project_dir/uninstall.txt"

OMR_SESSION_CAPTURE="$capture" go run ./cmd/omr session resume --project-dir "$project_dir" --binary "$fake_binary"
grep -qx -- '--continue' "$capture"
OMR_SESSION_CAPTURE="$capture" go run ./cmd/omr session resume --project-dir "$project_dir" --binary "$fake_binary" --copy
grep -qx -- '--continue' "$capture"
grep -qx -- '--copy' "$capture"

go run ./cmd/omr uninstall --project-dir "$project_dir" > "$project_dir/uninstall-real.txt"
test ! -e "$project_dir/.reasonix/omr/manifest.lock.yaml"
test ! -e "$project_dir/.reasonix/omr/generated/system-prompt.md"
! grep -q 'system_prompt_file' "$project_dir/reasonix.toml"

go run ./cmd/omr init --project-dir "$project_dir" >/dev/null
printf '\nmanual drift\n' >> "$project_dir/.reasonix/omr/generated/system-prompt.md"
if go run ./cmd/omr doctor --project-dir "$project_dir" --json > "$project_dir/drift.json"; then
  echo "expected generated Prompt drift to fail doctor" >&2
  exit 1
fi
grep -q 'generated Prompt hash drift detected' "$project_dir/drift.json"

mkdir -p "$project_dir/.reasonix/omr"
cat > "$project_dir/.reasonix/omr/config.toml" <<'EOF'
[runtime]
max_steps = 4
max_steps = 8
EOF
if go run ./cmd/omr config validate --config "$project_dir/.reasonix/omr/config.toml" --json > "$project_dir/invalid-config.json"; then
  echo "expected invalid config to fail" >&2
  exit 1
fi
grep -q '"valid":false' "$project_dir/invalid-config.json"
grep -q 'duplicate key' "$project_dir/invalid-config.json"

echo "OMR CLI smoke: PASS"
