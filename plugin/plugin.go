package plugin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/opdev/container-certification/internal/crane"
	"github.com/opdev/container-certification/internal/defaults"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/container-certification/internal/writer"
	"github.com/opdev/knex/plugin/v0"
	"github.com/opdev/knex/types"
	"github.com/spf13/viper"
)

var vers = semver.MustParse("0.0.1")

func init() {
	plugin.Register("check-container", NewPlugin())
}

type plug struct {
	writer.FileWriter

	image  string
	engine *crane.CraneEngine
}

func NewPlugin() *plug {
	return &plug{}
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
			policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
				defaults.DefaultPyxisHost,
				"", // TODO(Jose): Pyxis API Token stubbed for this PoC
				"", // TODO(Jose): Pyxis Project ID stubbed for this PoC
				&http.Client{Timeout: 60 * time.Second})),
			&policy.MaxLayersCheck{},
		},
		Platform:  "amd64",
		IsScratch: false,
		Insecure:  false,
	}
	return nil
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
