package plugin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/defaults"
	"github.com/opdev/container-certification/internal/flags"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
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

func (p *plug) Init(ctx context.Context, cfg *viper.Viper, args []string) error {
	l := logr.FromContextOrDiscard(ctx) // TODO(Jose): Do we want to provide an equivalent function within preflight so plugins don't have to pull this dependency?
	l.Info("Initializing Container Certification")
	if len(args) != 1 {
		return errors.New("a single argument is required (the container image to test)")
	}
	p.image = args[0]
	p.engine = &crane.CraneEngine{
		DockerConfig: "",
		Image:        p.image,
		Checks: []types.Check{
			&policy.HasLicenseCheck{},
			policy.NewHasUniqueTagCheck(cfg.GetString("docker-config")), // TODO(Jose): DockerConfigPath stubbed for this PoC
			&policy.MaxLayersCheck{},
			&policy.HasNoProhibitedPackagesCheck{},
			&policy.HasRequiredLabelsCheck{},
			&policy.RunAsNonRootCheck{},
			&policy.HasModifiedFilesCheck{},
			policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
				defaults.DefaultPyxisHost,
				"", // TODO(Jose): Pyxis API Token stubbed for this PoC
				"", // TODO(Jose): Pyxis Project ID stubbed for this PoC
				&http.Client{Timeout: 60 * time.Second})),
		},
		Platform:  "amd64",
		IsScratch: false,
		Insecure:  false,
	}
	return nil
}

func (p *plug) BindFlags(f *pflag.FlagSet) *pflag.FlagSet {
	flags.BindFlagDockerConfigFilePath(f)
	flags.BindFlagPyxisAPIToken(f)
	flags.BindFlagPyxisEnv(f)
	flags.BindFlagPyxisHost(f)
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
	return nil
}
