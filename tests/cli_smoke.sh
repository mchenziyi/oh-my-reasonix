#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
project_dir=$(mktemp -d "${TMPDIR:-/tmp}/omr-cli-smoke.XXXXXX")
trap 'rm -rf "$project_dir"' EXIT

printf '[agent]\n' > "$project_dir/reasonix.toml"

cd "$repo_dir"
go run ./cmd/omr init --project-dir "$project_dir" >/dev/null
mkdir -p "$project_dir/.reasonix/omr"
: > "$project_dir/.reasonix/omr/config.toml"
go run ./cmd/omr doctor --project-dir "$project_dir" --json > "$project_dir/doctor.json"
go run ./cmd/omr profile list --project-dir "$project_dir" --json > "$project_dir/profiles.json"
go run ./cmd/omr config validate --config "$project_dir/.reasonix/omr/config.toml" --json > "$project_dir/config.json"

grep -q '"name":"manifest"' "$project_dir/doctor.json"
grep -q '"id":"omr-explore"' "$project_dir/profiles.json"
grep -q '"id":"omr-research"' "$project_dir/profiles.json"
grep -q '"id":"omr-debug"' "$project_dir/profiles.json"
grep -q '"valid":true' "$project_dir/config.json"

echo "OMR CLI smoke: PASS"
