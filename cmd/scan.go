package cmd

import (
	"context"
	"fmt"

	awsprovider "github.com/esanchezm/terradrift/internal/provider/aws"
	"github.com/esanchezm/terradrift/internal/provider"
	"github.com/spf13/cobra"
)

var (
	scanProvider string
	scanType    string
	scanRegion  string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for infrastructure drift",
	Long: `Scan your cloud infrastructure for drift against your IaC configuration.

This command compares your actual cloud resource state against your
Terraform or other IaC configuration and reports any differences.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if scanProvider == "" {
			return fmt.Errorf("provider is required (use --provider)")
		}

		ctx := context.Background()
		prov, err := newProvider(ctx, scanProvider, scanRegion)
		if err != nil {
			return fmt.Errorf("failed to create provider: %w", err)
		}

		var types []string
		if scanType != "" {
			types = []string{scanType}
		}

		resources, err := prov.Resources(ctx, types)
		if err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		fmt.Printf("Found %d resources\n", len(resources))
		for _, r := range resources {
			fmt.Printf("  - %s (%s)\n", r.ID, r.Type)
		}

		return nil
	},
}

func newProvider(ctx context.Context, name, region string) (provider.Provider, error) {
	switch name {
	case "aws":
		return awsprovider.New(ctx, region)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&scanProvider, "provider", "", "Cloud provider (aws, gcp, azure)")
	scanCmd.Flags().StringVar(&scanType, "type", "", "Resource type (e.g., aws_instance)")
	scanCmd.Flags().StringVar(&scanRegion, "region", "", "Cloud region")
}