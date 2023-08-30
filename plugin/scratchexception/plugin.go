package plugin

import (
	"context"
	"errors"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/opdev/container-certification/internal/checks"
	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/flags"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/knex/plugin/v0"
	"github.com/opdev/knex/types"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Assert that we implement the Plugin interface.
var _ plugin.Plugin = NewPlugin()

var vers = semver.MustParse("0.0.1")

func init() {
	plugin.Register("check-container-scratch-exception-policy", NewPlugin())
}

type plug struct {
	image  string
	engine *crane.CraneEngine
}

func NewPlugin() *plug {
	p := plug{}
	// plugin-related things may happen here.
	return &p
}

func (p *plug) Register() error {
	return nil
}

func (p *plug) Name() string {
	return "Container Certification (Scratch Exception)"
}

func (p *plug) Init(ctx context.Context, cfg *viper.Viper, args []string) error {
	l := logr.FromContextOrDiscard(ctx) // TODO(Jose): Do we want to provide an equivalent function within preflight so plugins don't have to pull this dependency?
	l.Info("Initializing Container Certification - Scratch Exception Policy")
	if len(args) != 1 {
		return errors.New("a single argument is required (the container image to test)")
	}

	pol := policy.PolicyScratch

	renderedChecks, err := checks.InitializeContainerChecks(ctx, pol, checks.ContainerCheckConfig{
		DockerConfig:           cfg.GetString(flags.KeyDockerConfig),
		PyxisAPIToken:          cfg.GetString(flags.KeyPyxisAPIToken),
		CertificationProjectID: cfg.GetString(flags.KeyCertProjectID),
		PyxisHost:              cfg.GetString(flags.KeyPyxisHost),
	})
	if err != nil {
		return err
	}

	p.image = args[0]
	p.engine = &crane.CraneEngine{
		DockerConfig: cfg.GetString(flags.KeyDockerConfig),
		Image:        p.image,
		Checks:       renderedChecks,
		Platform:     cfg.GetString(flags.KeyPlatform),
		IsScratch:    pol == policy.PolicyScratch,
		Insecure:     false, // TOOD(Jose): This isn't wired because this probably needs to come from the preflight tool? Maybe not.
	}
	return nil
}

func (p *plug) BindFlags(f *pflag.FlagSet) *pflag.FlagSet {
	flags.BindFlagDockerConfigFilePath(f)
	flags.BindFlagPyxisAPIToken(f)
	flags.BindFlagPyxisEnv(f)
	flags.BindFlagPyxisHost(f)
	flags.BindFlagsImagePlatform(f)
	return f
}

func (p *plug) Version() semver.Version {
	return *vers
}

func (p *plug) ExecuteChecks(ctx context.Context) error {
	l := logr.FromContextOrDiscard(ctx)
	l.Info("Execute Checks Called")
	return p.engine.ExecuteChecks(ctx)
}

func (p *plug) Results(ctx context.Context) types.Results {
	return p.engine.Results(ctx)
}

func (p *plug) Submit(ctx context.Context) error {
	l := logr.FromContextOrDiscard(ctx)
	l.Info("Submission is not allowed for this plugin. Please use the container-certification plugin for submission.")
	return nil
}
