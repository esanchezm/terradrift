package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// scanCmd represents the scan command.
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for infrastructure drift",
	Long: `Scan your cloud infrastructure for drift against your IaC configuration.

This command compares your actual cloud resource state against your
Terraform or other IaC configuration and reports any differences.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("scan called")
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	// Here you will define your flags for this command.
	// cobraFlags are available in scanCmd.
}
