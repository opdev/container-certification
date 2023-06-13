package plugin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/defaults"
	"github.com/opdev/container-certification/internal/flags"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/container-certification/internal/writer"
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
	writer.FileWriter
	flags *pflag.FlagSet

	image  string
	engine *crane.CraneEngine
}

func NewPlugin() *plug {
	p := plug{}
	p.initializeFlagSet()
	return &p
}

func (p *plug) initializeFlagSet() {
	// NOTE(komish): This is borrowed from Cobra for this PoC.
	if p.flags == nil {
		p.flags = pflag.NewFlagSet(p.Name(), pflag.ContinueOnError)
		// if p.flagErrorBuf == nil {
		// 	p.flagErrorBuf = new(bytes.Buffer)
		// }
		// p.flags.SetOutput(p.flagErrorBuf)
	}

	flags.BindFlagDockerConfigFilePath(p.flags)
}

func (p *plug) Register() error {
	return nil
}

func (p *plug) Name() string {
	return "Container Certification"
}

func (p *plug) Init(cfg *viper.Viper) error {
	fmt.Println("Init called")
	p.image = "quay.io/opdev/simple-demo-operator:latest" // placeholder for testing
	p.engine = &crane.CraneEngine{
		DockerConfig: "",
		Image:        p.image,
		Checks: []types.Check{
			&policy.HasLicenseCheck{},
			policy.NewHasUniqueTagCheck(""), // TODO(Jose): DockerConfigPath stubbed for this PoC
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

func (p *plug) Flags() *pflag.FlagSet {
	return p.flags
}

func (p *plug) Version() semver.Version {
	return *vers
}

func (p *plug) ExecuteChecks(ctx context.Context) error {
	fmt.Println("Execute Checks Called")
	return p.engine.ExecuteChecks(ctx)
}

func (p *plug) Results(ctx context.Context) types.Results {
	return p.engine.Results(ctx)
}

func (p *plug) Submit(_ context.Context) error {
	fmt.Println("Submit called")
	return nil
}
