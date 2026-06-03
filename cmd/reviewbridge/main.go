package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ahmedennaime/reviewbridge/internal/config"
	"github.com/ahmedennaime/reviewbridge/internal/db"
)

var rootCmd = &cobra.Command{
	Use:   "reviewbridge",
	Short: "Routes PR/MR review comments into the right Claude Code session",
	Long: `ReviewBridge monitors GitHub PRs and GitLab MRs for review comments,
triages them with Claude, and routes approved fixes into the correct
Claude Code session — without breaking your current flow.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("reviewbridge v0.1.0")
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Guided first-time setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		r := &initRunner{
			prompter:   newStdinPrompter(os.Stdin, os.Stdout),
			validators: defaultInitValidators(),
			configPath: config.ConfigPath(),
			out:        os.Stdout,
		}
		return r.run()
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the ReviewBridge daemon in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStart(os.Stdout, defaultPIDPath(), defaultSpawner())
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the ReviewBridge daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStop(os.Stdout, defaultPIDPath())
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tracked sessions and their linked PRs/MRs",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := db.Open(config.DBPath())
		if err != nil {
			return err
		}
		defer d.Close()
		return runStatus(os.Stdout, d)
	},
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "View all pending and parked review comments",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := db.Open(config.DBPath())
		if err != nil {
			return err
		}
		defer d.Close()
		return runQueue(os.Stdout, d)
	},
}

var linkCmd = &cobra.Command{
	Use:   "link",
	Short: "Manually link a session to a branch or PR/MR",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID, _ := cmd.Flags().GetString("session")
		branch, _ := cmd.Flags().GetString("branch")
		prID, _ := cmd.Flags().GetString("pr")

		d, err := db.Open(config.DBPath())
		if err != nil {
			return err
		}
		defer d.Close()
		return runLink(os.Stdout, d, sessionID, branch, prID)
	},
}

var installSkillCmd = &cobra.Command{
	Use:   "install-skill",
	Short: "Install the /check-reviews Claude Code slash command",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installSkill(os.Stdout)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for a newer version of ReviewBridge",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("reviewbridge v0.1.0 — check https://github.com/ahmedennaime/reviewbridge/releases for updates")
	},
}

// daemonCmd is the internal command that runs the daemon in the foreground.
// It is started by "start" as a background subprocess.
var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the daemon in the foreground (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon(config.ConfigPath(), config.DBPath(), defaultPIDPath())
	},
}

func init() {
	linkCmd.Flags().String("session", "", "Session ID to link")
	linkCmd.Flags().String("branch", "", "Branch name to link the session to")
	linkCmd.Flags().String("pr", "", "PR/MR ID to link the session to")

	rootCmd.AddCommand(
		versionCmd,
		initCmd,
		startCmd,
		stopCmd,
		statusCmd,
		queueCmd,
		linkCmd,
		installSkillCmd,
		updateCmd,
		daemonCmd,
	)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
