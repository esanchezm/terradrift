package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "terradrift",
	Short: "terradrift detects infrastructure drift in your cloud environment",
	Long: `terradrift is a CLI tool that detects and reports infrastructure drift.

It compares your actual cloud infrastructure state against your desired 
Terraform/IaC configuration and reports any differences.`,
	Version: version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
}
