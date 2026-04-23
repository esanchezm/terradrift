package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/esanchezm/terradrift/internal/core"
	"github.com/esanchezm/terradrift/internal/diff"
	"github.com/esanchezm/terradrift/internal/output"
	"github.com/esanchezm/terradrift/internal/provider"
	awsprovider "github.com/esanchezm/terradrift/internal/provider/aws"
	"github.com/esanchezm/terradrift/internal/state"
	"github.com/esanchezm/terradrift/internal/state/factory"
)

var (
	scanProvider string
	scanType     string
	scanRegion   string
	scanState    string
	scanNoColor  bool
	scanQuiet    bool
)

// newProviderFn is a package-level indirection through which scanCmd
// obtains its provider. Tests override it to inject a fake provider
// without spinning up real cloud clients.
var newProviderFn = func(ctx context.Context, name, region string) (provider.Provider, error) {
	switch name {
	case "aws":
		return awsprovider.New(ctx, region)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for infrastructure drift",
	Long: `Scan your cloud infrastructure for drift against your IaC configuration.

This command loads Terraform state from the source given by --state, lists the
resources in the cloud via --provider / --region, and prints a
terraform-plan-style diff of the two. Output is colored by default; pass
--no-color for plain text or --quiet to suppress everything except the summary
line.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if scanProvider == "" {
			return fmt.Errorf("provider is required (use --provider)")
		}
		if scanState == "" {
			return fmt.Errorf("state source is required (use --state)")
		}

		ctx := cmd.Context()

		reader, err := factory.NewStateReader(ctx, scanState)
		if err != nil {
			return fmt.Errorf("failed to create state reader: %w", err)
		}

		prov, err := newProviderFn(ctx, scanProvider, scanRegion)
		if err != nil {
			return fmt.Errorf("failed to create provider: %w", err)
		}

		return runScan(ctx, scanConfig{
			Reader:       reader,
			Provider:     prov,
			TypeFilter:   scanType,
			ProviderName: scanProvider,
			Region:       scanRegion,
			NoColor:      scanNoColor,
			Quiet:        scanQuiet,
			Out:          cmd.OutOrStdout(),
		})
	},
}

// scanConfig bundles the inputs runScan needs. Grouping them in a struct
// keeps the function signature stable as new flags land and lets tests
// fill only the fields they exercise.
type scanConfig struct {
	Reader       state.StateReader
	Provider     provider.Provider
	TypeFilter   string
	ProviderName string
	Region       string
	NoColor      bool
	Quiet        bool
	Out          io.Writer
}

// runScan is the testable core of the scan command. It validates
// cfg.TypeFilter against the provider's supported types, loads desired
// resources from cfg.Reader, queries the provider for actual resources
// (scoping the query to TypeFilter when set), filters the desired side
// by the same type so both halves of the diff have identical scope,
// calculates drift, and renders the report.
//
// A non-empty TypeFilter that the provider does not support is a hard
// error: silently returning a report with every state resource marked
// missing (because no matching cloud resources were fetched) would be
// worse than useless.
func runScan(ctx context.Context, cfg scanConfig) error {
	if cfg.TypeFilter != "" && !providerSupportsType(cfg.Provider, cfg.TypeFilter) {
		return fmt.Errorf("unsupported resource type %q for provider %q (supported: %v)",
			cfg.TypeFilter, cfg.Provider.Name(), cfg.Provider.SupportedTypes())
	}

	desired, err := cfg.Reader.Resources(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state resources: %w", err)
	}

	var types []string
	if cfg.TypeFilter != "" {
		types = []string{cfg.TypeFilter}
		desired = filterResourcesByType(desired, cfg.TypeFilter)
	}

	actual, err := cfg.Provider.Resources(ctx, types)
	if err != nil {
		return fmt.Errorf("failed to list cloud resources: %w", err)
	}

	report := diff.CalculateDrift(desired, actual)

	info := output.ScanInfo{
		Provider:           cfg.ProviderName,
		Region:             cfg.Region,
		StateSource:        cfg.Reader.Source(),
		StateResourceCount: len(desired),
	}
	renderer := output.New(output.Options{
		Writer:  cfg.Out,
		NoColor: cfg.NoColor,
		Quiet:   cfg.Quiet,
	})
	return renderer.Render(info, &report)
}

func providerSupportsType(p provider.Provider, t string) bool {
	for _, s := range p.SupportedTypes() {
		if s == t {
			return true
		}
	}
	return false
}

func filterResourcesByType(rs []core.Resource, t string) []core.Resource {
	out := make([]core.Resource, 0, len(rs))
	for _, r := range rs {
		if r.Type == t {
			out = append(out, r)
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(&scanProvider, "provider", "", "Cloud provider (aws, gcp, azure)")
	scanCmd.Flags().StringVar(&scanType, "type", "", "Resource type (e.g., aws_instance); must match one of the provider's supported types")
	scanCmd.Flags().StringVar(&scanRegion, "region", "", "Cloud region (e.g., eu-west-1)")
	scanCmd.Flags().StringVar(&scanState, "state", "", "State source: path to tfstate, s3://bucket/key, http(s):// URL, or - for stdin")
	scanCmd.Flags().BoolVar(&scanNoColor, "no-color", false, "Disable colored output (implies plain ASCII regardless of TTY)")
	scanCmd.Flags().BoolVar(&scanQuiet, "quiet", false, "Print only the summary line")
}
