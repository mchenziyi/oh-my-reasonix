package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenExistingMemoryStoreDoesNotCreateMissingStore(t *testing.T) {
	project := t.TempDir()
	if _, err := openExistingMemoryStore(project, "project"); err == nil {
		t.Fatal("missing memory store must fail")
	}
	if _, err := os.Stat(memoryStoreRoot(project)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only command created a memory store")
	}
}

func TestReadBoundedJSONFileRejectsUnsafeInput(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "context.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(link); err == nil {
		t.Fatal("symlink input must fail")
	}
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, []byte(strings.Repeat("x", 1<<20+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(large); err == nil {
		t.Fatal("oversized input must fail")
	}
}

func TestStrictJSONRejectsUnknownAndTrailingData(t *testing.T) {
	var doc episodicContextDocument
	if err := strictJSON([]byte(`{"schema_version":1,"unknown":true}`), &doc); err == nil {
		t.Fatal("unknown field must fail")
	}
	if err := strictJSON([]byte(`{"schema_version":1}{}`), &doc); err == nil {
		t.Fatal("trailing JSON must fail")
	}
}

func TestExtractStringFlagForms(t *testing.T) {
	for _, args := range [][]string{{"--episode-id", "episode_1", "--json"}, {"--episode-id=episode_1", "--json"}} {
		got, rest, err := extractStringFlag(args, "episode-id")
		if err != nil || got != "episode_1" || len(rest) != 1 || rest[0] != "--json" {
			t.Fatalf("unexpected result: %q %v %v", got, rest, err)
		}
	}
}

func TestMemoryCLIRejectsIncompleteRequests(t *testing.T) {
	if err := runMemory([]string{"episodic", "card", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing episode id must fail")
	}
	if err := runMemory([]string{"episodic", "validate-receipt", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing receipt file must fail")
	}
	if err := runMemory([]string{"usage", "capture", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing usage receipt files must fail")
	}
	if err := runMemory([]string{"outcome", "capture", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing attribution receipt must fail")
	}
}

func TestMemoryUsageCLIRejectsUnknownFieldsBeforeStoreAccess(t *testing.T) {
	dir := t.TempDir()
	librarian := filepath.Join(dir, "librarian.json")
	usage := filepath.Join(dir, "usage.json")
	if err := os.WriteFile(librarian, []byte(`{"schema_version":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usage, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"usage", "capture", "--project-dir", dir, "--librarian-receipt", librarian, "--usage-receipt", usage}); err == nil {
		t.Fatal("unknown field must fail")
	}
	if _, err := os.Stat(memoryStoreRoot(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid input created a memory store")
	}
}
