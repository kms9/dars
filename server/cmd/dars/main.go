package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
	"github.com/kms9/dars/internal/daemon/execenv"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// debugFlag is bound to the persistent --debug flag and, when set, makes
// FormatError emit the full original error chain instead of just the
// user-facing message.
var debugFlag bool

var rootCmd = &cobra.Command{
	Use:           "dars",
	Short:         "DARS CLI — local agent runtime and management tool",
	Long:          "Work seamlessly with DARS from the command line.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)\ngo: %s, os/arch: %s/%s", version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	rootCmd.SetVersionTemplate("dars {{.Version}}\n")

	// Tag every CLI HTTP request with this binary's build version so the
	// server can split logs/metrics by client version.
	cli.ClientVersion = version

	rootCmd.PersistentFlags().String("server-url", "", "DARS server URL (env: DARS_SERVER_URL)")
	rootCmd.PersistentFlags().String("workspace-id", "", "Workspace ID (env: DARS_WORKSPACE_ID)")
	rootCmd.PersistentFlags().String("profile", "", "Configuration profile name (e.g. dev) — isolates config, daemon state, and workspaces")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print full error details on failure (env: DARS_DEBUG)")

	// Lightweight product commands are the complete shipping command tree.
	issueCmd.GroupID = groupCore
	agentCmd.GroupID = groupCore
	workspaceCmd.GroupID = groupCore
	skillCmd.GroupID = groupCore
	squadCmd.GroupID = groupCore
	chatCmd.GroupID = groupCore

	// Runtime commands
	daemonCmd.GroupID = groupRuntime
	runtimeCmd.GroupID = groupRuntime

	// Additional commands
	authCmd.GroupID = groupAdditional
	userCmd.GroupID = groupAdditional
	loginCmd.GroupID = groupAdditional
	setupCmd.GroupID = groupAdditional
	configCmd.GroupID = groupAdditional
	versionCmd.GroupID = groupAdditional

	rootCmd.AddCommand(issueCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(skillCmd)
	rootCmd.AddCommand(squadCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(runtimeCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)

	initHelp(rootCmd)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == execenv.PreparationHelperArg {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		if err := execenv.RunPreparationHelper(os.Stdin, os.Stdout, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := rootCmd.Execute(); err != nil {
		if err != errSilent {
			fmt.Fprintln(os.Stderr, cli.FormatError(err, debugFlag))
		}
		os.Exit(cli.ExitCodeFor(err))
	}
}
