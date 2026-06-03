package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillCopiesFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, ".claude", "commands")
	os.MkdirAll(dest, 0755) //nolint:errcheck

	if err := installSkillTo(dest, &strings.Builder{}); err != nil {
		t.Fatalf("installSkillTo: %v", err)
	}

	path := filepath.Join(dest, "check-reviews.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("skill file not created: %v", err)
	}
}

func TestInstallSkillOverwritesExisting(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "check-reviews.md"), []byte("old content"), 0644) //nolint:errcheck

	if err := installSkillTo(dir, &strings.Builder{}); err != nil {
		t.Fatalf("installSkillTo: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "check-reviews.md"))
	if string(data) == "old content" {
		t.Error("skill file should have been overwritten with latest version")
	}
}

func TestInstallSkillCreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "commands")

	if err := installSkillTo(dir, &strings.Builder{}); err != nil {
		t.Fatalf("installSkillTo: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should have been created: %v", err)
	}
}
