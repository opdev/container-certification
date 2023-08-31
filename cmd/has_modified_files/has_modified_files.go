package main

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/opdev/container-certification/internal/cli"
	"github.com/opdev/container-certification/internal/policy"
)

func main() {
	cmd := hasModifiedFilesCmd()
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func hasModifiedFilesCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:  "has-modified-files",
		Args: cobra.MinimumNArgs(1),
		Long: `Run the "HasModifiedFiles" check of Red Hat's Container Certification Policy. This is a debugging tool, and not used for certification. This tool is intended to allow developers to run individual checks of container certification as they develop their images.`,
		RunE: cli.RunEFunctionWithCheck(&policy.HasModifiedFilesCheck{}),
	}

	f := cmd.Flags()
	cli.BindBaseFlags(f)

	return &cmd
}
