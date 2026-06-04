package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/config"
	"github.com/ahmedennaime/reviewbridge/internal/daemon"
	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/notify"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	github_pkg "github.com/ahmedennaime/reviewbridge/internal/platforms/github"
	gitlab_pkg "github.com/ahmedennaime/reviewbridge/internal/platforms/gitlab"
	"github.com/ahmedennaime/reviewbridge/internal/poller"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/queuefile"
	"github.com/ahmedennaime/reviewbridge/internal/session"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

const daemonNotRunningMsg = "ReviewBridge daemon is not running"

func defaultPIDPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".reviewbridge", "daemon.pid")
}

func defaultLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".reviewbridge", "daemon.log")
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func runStart(out io.Writer, pidPath string, spawner func() error) error {
	if pid, err := readPID(pidPath); err == nil && isProcessAlive(pid) {
		fmt.Fprintln(out, "ReviewBridge daemon is already running")
		return nil
	}
	if err := spawner(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	for range 20 {
		time.Sleep(100 * time.Millisecond)
		if pid, err := readPID(pidPath); err == nil && isProcessAlive(pid) {
			fmt.Fprintf(out, "ReviewBridge daemon started (pid %d)\n", pid)
			fmt.Fprintf(out, "Logs: %s\n", defaultLogPath())
			return nil
		}
	}
	fmt.Fprintln(out, "ReviewBridge daemon started")
	fmt.Fprintf(out, "Logs: %s\n", defaultLogPath())
	return nil
}

func defaultSpawner() func() error {
	return func() error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		logPath := defaultLogPath()
		if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
			return err
		}
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, "daemon")
		cmd.Stdin = nil
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		return cmd.Start()
	}
}


func runStop(out io.Writer, pidPath string) error {
	pid, err := readPID(pidPath)
	if err != nil {
		fmt.Fprintln(out, daemonNotRunningMsg)
		return nil
	}
	if !isProcessAlive(pid) {
		os.Remove(pidPath)
		fmt.Fprintln(out, daemonNotRunningMsg)
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintln(out, daemonNotRunningMsg)
		return nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}
	for range 30 {
		time.Sleep(100 * time.Millisecond)
		if _, statErr := os.Stat(pidPath); os.IsNotExist(statErr) {
			fmt.Fprintln(out, "ReviewBridge daemon stopped")
			return nil
		}
	}
	os.Remove(pidPath)
	fmt.Fprintln(out, "ReviewBridge daemon stopped")
	return nil
}

func runDaemon(configPath, dbPath, pidPath string) error {
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	d, err := db.Open(dbPath)
	if err != nil {
		return err
	}

	interval := cfg.Platforms.GitHub.PollingInterval
	if interval == 0 {
		interval = config.DefaultPollingInterval
	}

	plats := buildPlatforms(cfg)
	qfw := queuefile.New(expandHomePath(config.QueueDir()), d)
	q := queue.New(d)
	p := poller.New(d, plats, interval)
	t := triage.New(cfg.AnthropicAPIKey, d)
	n := notify.New()
	reg := session.NewRegistry(d)

	sessionsPath := expandHomePath(cfg.ClaudeCode.SessionsPath)

	deps := daemon.Deps{
		DB:           d,
		Poller:       p,
		Triage:       t,
		Queue:        q,
		QueueWriter:  qfw,
		Notifier:     n,
		Registry:     reg,
		Platforms:    plats,
		SessionsPath: sessionsPath,
	}

	dm := daemon.New(deps, pidPath)
	return dm.Run()
}

func buildPlatforms(cfg *config.Config) map[string]platforms.Platform {
	plats := make(map[string]platforms.Platform)
	if cfg.Platforms.GitHub.Token != "" {
		plats["github"] = github_pkg.New(cfg.Platforms.GitHub.Token, cfg.Platforms.GitHub.BaseURL)
	}
	if cfg.Platforms.GitLab.Token != "" {
		plats["gitlab"] = gitlab_pkg.New(cfg.Platforms.GitLab.Token, cfg.Platforms.GitLab.URL)
	}
	return plats
}

func expandHomePath(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
