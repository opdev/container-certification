package main

import (
	"log"

	"github.com/opdev/container-certification/internal/cli"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/spf13/cobra"
)

func main() {
	cmd := hasProhibitedPackagesCmd()
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func hasProhibitedPackagesCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:  "has-prohibited-packages",
		Args: cobra.MinimumNArgs(1),
		Long: `Run the "hasProhibitedPackages" check of Red Hat's Container Certification Policy. This is a debugging tool, and not used for certification. This tool is intended to allow developers to run individual checks of container certification as they develop their images.`,
		RunE: cli.RunEFunctionWithCheck(&policy.HasNoProhibitedPackagesCheck{}),
	}

	f := cmd.Flags()
	cli.BindBaseFlags(f)

	return &cmd
}
