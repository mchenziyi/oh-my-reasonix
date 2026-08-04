package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// makeTestKeyPairPEM writes a fresh ed25519 key pair as PEM files inside dir
// and returns (private path, public path). Used for signed-package tests.
func makeTestKeyPairPEM(t *testing.T, dir string) (string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privPEM, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	privPath := filepath.Join(dir, "sign-key.pem")
	privBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privPEM})
	if err := os.WriteFile(privPath, privBlock, 0o600); err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(dir, "sign-key.pub.pem")
	pubBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := os.WriteFile(pubPath, pubBlock, 0o600); err != nil {
		t.Fatal(err)
	}
	return privPath, pubPath
}

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
