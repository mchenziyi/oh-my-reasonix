#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
project_dir=$(mktemp -d "${TMPDIR:-/tmp}/omr-cli-smoke.XXXXXX")
fake_binary="$project_dir/fake-reasonix"
capture="$project_dir/session-args.txt"
trap 'rm -rf "$project_dir"' EXIT

printf '[agent]\n' > "$project_dir/reasonix.toml"
cat > "$fake_binary" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$OMR_SESSION_CAPTURE"
EOF
chmod +x "$fake_binary"

cd "$repo_dir"
go run ./cmd/omr init --project-dir "$project_dir" >/dev/null
mkdir -p "$project_dir/.reasonix/omr"
cat > "$project_dir/.reasonix/omr/config.toml" <<'EOF'
[agent.omr-research]
model = "deepseek-v4-flash"
read_only = true
EOF
go run ./cmd/omr doctor --project-dir "$project_dir" --json > "$project_dir/doctor.json"
go run ./cmd/omr profile list --project-dir "$project_dir" --json > "$project_dir/profiles.json"
go run ./cmd/omr config validate --config "$project_dir/.reasonix/omr/config.toml" --json > "$project_dir/config.json"
go run ./cmd/omr upgrade --project-dir "$project_dir" --dry-run > "$project_dir/upgrade.txt"
go run ./cmd/omr uninstall --project-dir "$project_dir" --dry-run > "$project_dir/uninstall.txt"

grep -q '"name":"manifest"' "$project_dir/doctor.json"
grep -q '"id":"omr-explore"' "$project_dir/profiles.json"
grep -q '"id":"omr-research"' "$project_dir/profiles.json"
grep -q '"id":"omr-debug"' "$project_dir/profiles.json"
grep -q '"model":"deepseek-v4-flash"' "$project_dir/profiles.json"
grep -q '"valid":true' "$project_dir/config.json"
grep -q 'NOOP\|PLAN' "$project_dir/upgrade.txt"
grep -q 'PLAN\|REMOVE' "$project_dir/uninstall.txt"

OMR_SESSION_CAPTURE="$capture" go run ./cmd/omr session resume --project-dir "$project_dir" --binary "$fake_binary"
grep -qx -- '--continue' "$capture"
OMR_SESSION_CAPTURE="$capture" go run ./cmd/omr session resume --project-dir "$project_dir" --binary "$fake_binary" --copy
grep -qx -- '--continue' "$capture"
grep -qx -- '--copy' "$capture"

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
