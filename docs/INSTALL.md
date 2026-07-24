# Install oh-my-reasonix from Reasonix

This document is written for a Reasonix agent. Install OMR only in the current
project; do not modify the user's global Reasonix configuration, API keys, or
the Reasonix binary.

## Rules

1. Work from the project root. If the project root cannot be identified, stop
   and ask the user for it.
2. Read `reasonix.toml` before changing anything.
3. Use a pinned OMR release and verify `SHA256SUMS` before running a downloaded
   binary.
4. Run the dry-run first and show the complete plan. Do not continue after a
   conflict.
5. If an existing `agent.system_prompt` or `agent.system_prompt_file` contains
   user content, ask the user before using
   `--compose-prompt --allow-persist-user-prompt`.
6. Keep the OMR binary in the project-local `.reasonix/omr/bin/` directory
   unless the user explicitly asks for a different location.

## 1. Select the OMR executable

Prefer an existing `omr` executable in `PATH` when its version matches the
requested release:

```bash
if command -v omr >/dev/null 2>&1; then
  OMR_BIN="$(command -v omr)"
  "$OMR_BIN" version
fi
```

If this repository is already checked out and Go 1.23 or newer is available,
the source fallback is:

```bash
go run ./cmd/omr version
```

When using that fallback, replace `"$OMR_BIN"` in the later commands with
`go run ./cmd/omr`.

Otherwise download the pinned release. Set `OMR_VERSION` to the version the
user requested; the example below is the current project version.

```bash
OMR_VERSION=v1.1.2
OMR_REPO=mchenziyi/oh-my-reasonix
mkdir -p .reasonix/omr/bin
```

On macOS or Linux, select the release asset using the host OS and architecture:

```bash
case "$(uname -s):$(uname -m)" in
  Darwin:arm64)  OMR_ASSET="omr-${OMR_VERSION}-darwin-arm64" ;;
  Darwin:x86_64) OMR_ASSET="omr-${OMR_VERSION}-darwin-amd64" ;;
  Linux:x86_64)  OMR_ASSET="omr-${OMR_VERSION}-linux-amd64" ;;
  Linux:aarch64) OMR_ASSET="omr-${OMR_VERSION}-linux-arm64" ;;
  *) echo "unsupported platform; ask the user to install OMR manually" >&2; exit 1 ;;
esac
curl --fail --location --silent --show-error \
  "https://github.com/${OMR_REPO}/releases/download/${OMR_VERSION}/${OMR_ASSET}" \
  --output ".reasonix/omr/bin/${OMR_ASSET}"
curl --fail --location --silent --show-error \
  "https://github.com/${OMR_REPO}/releases/download/${OMR_VERSION}/SHA256SUMS" \
  --output ".reasonix/omr/bin/SHA256SUMS"
EXPECTED="$(awk -v file="${OMR_ASSET}" '$2 == file {print $1}' .reasonix/omr/bin/SHA256SUMS)"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum ".reasonix/omr/bin/${OMR_ASSET}" | awk '{print $1}')"
else
  ACTUAL="$(shasum -a 256 ".reasonix/omr/bin/${OMR_ASSET}" | awk '{print $1}')"
fi
test -n "${EXPECTED}" && test "${ACTUAL}" = "${EXPECTED}"
chmod 0755 ".reasonix/omr/bin/${OMR_ASSET}"
OMR_BIN=".reasonix/omr/bin/${OMR_ASSET}"
```

On Windows PowerShell, use the matching `windows-amd64.exe` asset and verify it
before executing it:

```powershell
$OMR_VERSION = "v1.1.2"
$OMR_REPO = "mchenziyi/oh-my-reasonix"
$OMR_ASSET = "omr-$OMR_VERSION-windows-amd64.exe"
$OMR_DIR = ".reasonix/omr/bin"
New-Item -ItemType Directory -Force $OMR_DIR | Out-Null
$OMR_BIN = Join-Path $OMR_DIR $OMR_ASSET
$SUMS = Join-Path $OMR_DIR "SHA256SUMS"
Invoke-WebRequest "https://github.com/$OMR_REPO/releases/download/$OMR_VERSION/$OMR_ASSET" -OutFile $OMR_BIN
Invoke-WebRequest "https://github.com/$OMR_REPO/releases/download/$OMR_VERSION/SHA256SUMS" -OutFile $SUMS
$expected = ((Get-Content $SUMS | Where-Object { $_ -match "\s$([regex]::Escape($OMR_ASSET))$" }) -split "\s+")[0]
$actual = (Get-FileHash -Algorithm SHA256 $OMR_BIN).Hash.ToLowerInvariant()
if ([string]::IsNullOrWhiteSpace($expected) -or $actual -ne $expected.ToLowerInvariant()) { throw "OMR checksum verification failed" }
```

## 2. Install into the project

Use the selected executable (or the source fallback) for each command below.
The source fallback is equivalent to replacing `$OMR_BIN` with `go run
./cmd/omr`.

```bash
"$OMR_BIN" init --project-dir . --dry-run
```

Review the plan. Unless the user has explicitly approved it, stop here and ask
for confirmation. After confirmation:

```bash
"$OMR_BIN" init --project-dir .
```

If the dry-run reports existing user Prompt content, ask for explicit approval
before running:

```bash
"$OMR_BIN" init --project-dir . \
  --compose-prompt --allow-persist-user-prompt
```

The installation is project-scoped and should create or update only:

```text
reasonix.toml
.reasonix/omr/generated/system-prompt.md
.reasonix/omr/manifest.lock.yaml
.reasonix/omr/backups/<install-id>/reasonix.toml
.reasonix/skills/omr-explore/SKILL.md
```

## 3. Verify

```bash
"$OMR_BIN" doctor --project-dir .
"$OMR_BIN" config validate --project-dir .
```

Report the result, including any warnings. Do not claim success when doctor
reports an error.

## Optional Web/Docs MCP

OMR does not install, start, or authorize MCP servers. An optional server can
be declared in `.reasonix/omr/config.toml` for redacted compatibility
diagnostics and Profile guidance:

```toml
[mcp.docs]
transport = "stdio"
command = "mcp-docs"
capabilities = ["docs"]
enabled = false
env = ["DOCS_API_KEY"]
```

Keep it disabled until the user has installed and reviewed the server. To make
the tools available in a Reasonix session, register the same server through
Reasonix itself and review its exact command or endpoint:

```bash
reasonix mcp add docs mcp-docs
reasonix mcp list
```

Do not place tokens or `KEY=value` strings in the OMR `env` list. OMR stores
environment variable names only and never prints their values.

## Upgrade or uninstall

For an upgrade, download and verify the requested release first, then run:

```bash
"$OMR_BIN" upgrade --project-dir . --dry-run
"$OMR_BIN" upgrade --project-dir .
```

For removal, show the plan and then run:

```bash
"$OMR_BIN" uninstall --project-dir . --dry-run
"$OMR_BIN" uninstall --project-dir .
```

Never remove files that are not claimed by the OMR manifest or overwrite a
modified user-owned file. The project-local helper binary is intentionally
retained by `omr uninstall`; remove it separately only after confirming that
it is the OMR binary downloaded by this procedure.
