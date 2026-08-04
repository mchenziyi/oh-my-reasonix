package main

import (
	"os"
	"path/filepath"
	"testing"
)

// appendFileBytes appends raw bytes to a file for test corruption scenarios.
func appendFileBytes(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

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
		echo '{"schema_version":1,"command":"hook.list","hooks":[{"event":"PreToolUse","match":"Bash","scope":"project","status":"active"}]}'
		;;
	*"hook status"*)
		echo '{"schema_version":1,"command":"hook.status","trusted_project":true,"project_defines":true,"sources":[{"scope":"project","status":"loaded","hook_count":1}]}'
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
