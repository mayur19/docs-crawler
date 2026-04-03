package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/mayur19/docs-crawler/internal/discover"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/scope"
	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:   "discover [url]",
	Short: "Discover documentation URLs without fetching content",
	Long:  `Discover all documentation URLs from a seed URL and print them without crawling.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDiscover,
}

func init() {
	rootCmd.AddCommand(discoverCmd)

	discoverCmd.Flags().Int("max-depth", 0, "Max link depth (0 = unlimited)")
	discoverCmd.Flags().StringSlice("include", nil, "URL include glob patterns")
	discoverCmd.Flags().StringSlice("exclude", nil, "URL exclude glob patterns")
	discoverCmd.Flags().String("user-agent", "docs-crawler/0.1.0", "Custom User-Agent string")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	seedURL := args[0]
	parsed, err := url.Parse(seedURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	flags := cmd.Flags()
	maxDepth, _ := flags.GetInt("max-depth")
	includes, _ := flags.GetStringSlice("include")
	excludes, _ := flags.GetStringSlice("exclude")

	s := scope.NewScope(scope.ScopeConfig{
		Prefix:     seedURL,
		Includes:   includes,
		Excludes:   excludes,
		MaxDepth:   maxDepth,
		SameDomain: true,
	})

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	sitemapDisc := discover.NewSitemapDiscoverer(s)
	discoverers := []pipeline.Discoverer{sitemapDisc}

	count := 0
	for _, disc := range discoverers {
		ch, err := disc.Discover(ctx, parsed)
		if err != nil {
			slog.Warn("discoverer failed", "name", disc.Name(), "error", err)
			continue
		}
		for cu := range ch {
			fmt.Println(cu.String())
			count++
		}
	}

	fmt.Printf("\nTotal URLs found: %d\n", count)
	slog.Info("discovery complete", "urls_found", count)
	return nil
}
