package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStartWritesPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	var out strings.Builder

	spawner := func() error {
		return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600)
	}

	if err := runStart(&out, pidPath, spawner); err != nil {
		t.Fatalf("runStart: %v", err)
	}

	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("PID file not created: %v", err)
	}
}

func TestStartFailsIfAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")

	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600) //nolint:errcheck

	var out strings.Builder
	spawnerCalled := false
	spawner := func() error {
		spawnerCalled = true
		return nil
	}

	if err := runStart(&out, pidPath, spawner); err != nil {
		t.Fatalf("runStart: %v", err)
	}

	if spawnerCalled {
		t.Error("spawner should not have been called when daemon is already running")
	}
	if !strings.Contains(out.String(), "already running") {
		t.Errorf("expected 'already running' message, got: %q", out.String())
	}
}

func TestStopKillsDaemon(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subprocess: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() }) //nolint:errcheck

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600) //nolint:errcheck

	var out strings.Builder
	if err := runStop(&out, pidPath); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	if !strings.Contains(out.String(), "stopped") {
		t.Errorf("expected 'stopped' message, got: %q", out.String())
	}

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should have been removed after stop")
	}
}

func TestStopFailsIfNotRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")

	var out strings.Builder
	if err := runStop(&out, pidPath); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	if !strings.Contains(out.String(), "not running") {
		t.Errorf("expected 'not running' message, got: %q", out.String())
	}
}
