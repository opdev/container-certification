// package cli facilitates the generation of various CLI callables.
package cli

import (
	"context"
	"fmt"

	"github.com/bombsimon/logrusr/v4"
	"github.com/go-logr/logr"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/x/plugin/v0"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/flags"
)

type cobraRunEFunc = func(cmd *cobra.Command, args []string) error

// RunEFunctionWithCheck is a minimal execution of the container policy to be
// used for individual checks contained in debugging binaries.
func RunEFunctionWithCheck(ch plugin.Check) cobraRunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		ctx := configureLoggerAndStuffInto(cmd.Context())
		checks := []plugin.Check{ch}
		// TODO(Jose): Should we just rely in a viper config instead, and let each caller bind their own flags?
		dockerCfg, _ := cmd.Flags().GetString(flags.KeyDockerConfig)
		platform, _ := cmd.Flags().GetString(flags.KeyPlatform)

		engine := &crane.CraneEngine{
			DockerConfig: dockerCfg,
			Image:        args[0],
			Checks:       checks,
			Platform:     platform,
			IsScratch:    false,
			Insecure:     false, // TODO(Jose): This isn't wired because this probably needs to come from the preflight tool? Maybe not.
		}

		if err := engine.ExecuteChecks(ctx); err != nil {
			return err
		}

		results := engine.Results(ctx)
		textResults, _ := formatAsText(ctx, results)

		fmt.Fprintln(cmd.OutOrStdout(), string(textResults))
		return nil
	}
}

type FormatterFunc = func(context.Context, plugin.Results) (response []byte, formattingError error)

// Just as poc formatter, borrowed from preflight's library docs
var formatAsText FormatterFunc = func(_ context.Context, r plugin.Results) (response []byte, formattingError error) {
	b := []byte{}
	for _, v := range r.Passed {
		t := v.ElapsedTime.Milliseconds()
		s := fmt.Sprintf("PASSED  %s in %dms\n", v.Check.Name(), t)
		b = append(b, []byte(s)...)
	}
	for _, v := range r.Failed {
		t := v.ElapsedTime.Milliseconds()
		s := fmt.Sprintf("FAILED  %s in %dms\n", v.Check.Name(), t)
		b = append(b, []byte(s)...)
	}
	for _, v := range r.Errors {
		t := v.ElapsedTime.Milliseconds()
		s := fmt.Sprintf("ERRORED %s in %dms\n", v.Check.Name(), t)
		b = append(b, []byte(s)...)
	}

	return b, nil
}

// BindBaseFlags binds flags expected by this package's RunEFunctionWithCheck function.
func BindBaseFlags(f *pflag.FlagSet) {
	flags.BindFlagDockerConfigFilePath(f)
	flags.BindFlagsImagePlatform(f)
}

func configureLoggerAndStuffInto(ctx context.Context) context.Context {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{DisableColors: true})
	l.SetLevel(logrus.TraceLevel)
	logger := logrusr.New(l)
	return logr.NewContext(ctx, logger.WithValues("emitter", "debug binary"))
}
