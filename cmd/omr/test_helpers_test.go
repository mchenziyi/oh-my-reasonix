package main

import (
	"os"
	"path/filepath"
	"testing"
)

// makeMockReasonixBinary creates a bash script that returns canned JSON
// responses based on the CLI args it receives. This keeps hook doctor tests
// independent of the real Reasonix binary and ~/.reasonix permissions.
func makeMockReasonixBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "reasonix")
	script := `#!/bin/bash
set -e
case "$*" in
	*"hook list"*)
		echo '{"hooks":[{"name":"test-hook","status":"active","event":"commit","scope":"local"}],"schema_version":1}'
		;;
	*"hook status"*)
		echo '{"active":[{"name":"test-hook"}],"inactive":[],"untrusted":[],"schema_version":1}'
		;;
	*)
		echo '{"error":"unexpected args: $*"}' >&2
		exit 1
		;;
esac
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
