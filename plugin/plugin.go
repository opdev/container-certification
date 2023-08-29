package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/opdev/container-certification/internal/config"
	preflighterr "github.com/redhat-openshift-ecosystem/openshift-preflight/errors"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/opdev/container-certification/internal/checks"
	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/exceptions"
	"github.com/opdev/container-certification/internal/flags"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/container-certification/internal/submit"
	"github.com/opdev/knex/plugin/v0"
	"github.com/opdev/knex/types"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Assert that we implement the Plugin interface.
var _ plugin.Plugin = NewPlugin()

var vers = semver.MustParse("0.0.1")

func init() {
	plugin.Register("check-container", NewPlugin())
}

type plug struct {
	config *viper.Viper
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
	return "Container Certification"
}

func (p *plug) hasPyxisData(cfg *viper.Viper) bool {
	return cfg.GetString(flags.KeyPyxisAPIToken) != "" && cfg.GetString(flags.KeyCertProjectID) != ""
}

func (p *plug) Init(ctx context.Context, cfg *viper.Viper, args []string) error {
	l := logr.FromContextOrDiscard(ctx) // TODO(Jose): Do we want to provide an equivalent function within preflight so plugins don't have to pull this dependency?
	l.Info("Initializing Container Certification")
	if len(args) != 1 {
		return errors.New("a single argument is required (the container image to test)")
	}

	//Note(Jose): This is policy resolution code is ripped directly from the Preflight library code.
	pol := policy.PolicyContainer

	// determining the pyxis host based on the flags passed in at runtime
	pyxisHost := config.PyxisHostLookup(cfg.GetString(flags.KeyPyxisEnv), cfg.GetString(flags.KeyPyxisHost))

	// If we have enough Pyxis information, resolve the policy.
	if p.hasPyxisData(cfg) {
		pyxisClient := pyxis.NewPyxisClient(
			pyxisHost,
			cfg.GetString(flags.KeyPyxisAPIToken),
			cfg.GetString(flags.KeyCertProjectID),
			&http.Client{Timeout: 60 * time.Second},
		)

		override, err := exceptions.GetContainerPolicyExceptions(ctx, pyxisClient)
		if err != nil {
			return fmt.Errorf("%w: %s", preflighterr.ErrCannotResolvePolicyException, err)
		}

		pol = override
	} else {
		l.Info("Unable to get policy exceptions for this image because project information was not provided. Proceeding with default container policy.")
	}

	renderedChecks, err := checks.InitializeContainerChecks(ctx, pol, checks.ContainerCheckConfig{
		DockerConfig:           cfg.GetString(flags.KeyDockerConfig),
		PyxisAPIToken:          cfg.GetString(flags.KeyPyxisAPIToken),
		CertificationProjectID: cfg.GetString(flags.KeyCertProjectID),
		PyxisHost:              pyxisHost,
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
	flags.BindFlagCertificationProjectID(f)
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
	l.Info("Submit called")
	container := submit.ContainerCertificationSubmitter{
		CertificationProjectID: p.config.GetString(flags.KeyCertProjectID),
		Pyxis: submit.NewPyxisClient(
			ctx,
			p.config.GetString(flags.KeyCertProjectID),
			p.config.GetString(flags.KeyPyxisAPIToken),
			p.config.GetString(flags.KeyPyxisHost),
		),
		DockerConfig:     p.config.GetString(flags.KeyDockerConfig),
		PreflightLogFile: "preflight.log", // TODO: This is probably coming from knex so we need to map this somehow.
		PyxisEnv:         p.config.GetString(flags.KeyPyxisEnv),
	}

	return container.Submit(ctx)
}
