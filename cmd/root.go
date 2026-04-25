package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

// rootCmd represents the base command when called without any subcommands.
//
// SilenceUsage and SilenceErrors are set so cobra does not print its
// own diagnostics — handleExitError is the single owner of stderr.
// Without these, an unknown command or flag at the root level would
// produce both cobra's "Error: ..." plus a usage dump AND our own
// "Error: ..." line, which is noisy and confusing.
var rootCmd = &cobra.Command{
	Use:   "terradrift",
	Short: "terradrift detects infrastructure drift in your cloud environment",
	Long: `terradrift is a CLI tool that detects and reports infrastructure drift.

It compares your actual cloud infrastructure state against your desired 
Terraform/IaC configuration and reports any differences.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root cobra command and returns the process exit code
// for main to hand to os.Exit. Stderr rendering and exit-code
// classification live in handleExitError so they can be unit-tested
// without spawning a subprocess.
func Execute() int {
	return handleExitError(rootCmd.Execute(), os.Stderr)
}
