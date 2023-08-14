package checks

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/knex/types"
)

// Note(Jose): This is ripped directly from internal/engine code

// ContainerCheckConfig contains configuration relevant to an individual check's execution.
type ContainerCheckConfig struct {
	DockerConfig, PyxisAPIToken, CertificationProjectID, PyxisHost string
}

// InitializeContainerChecks returns the appropriate checks for policy p given cfg.
func InitializeContainerChecks(_ context.Context, p policy.Policy, cfg ContainerCheckConfig) ([]types.Check, error) {
	switch p {
	case policy.PolicyContainer:
		return []types.Check{
			&policy.HasLicenseCheck{},
			policy.NewHasUniqueTagCheck(cfg.DockerConfig),
			&policy.MaxLayersCheck{},
			&policy.HasNoProhibitedPackagesCheck{},
			&policy.HasRequiredLabelsCheck{},
			&policy.RunAsNonRootCheck{},
			&policy.HasModifiedFilesCheck{},
			policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
				cfg.PyxisHost,
				cfg.PyxisAPIToken,
				cfg.CertificationProjectID,
				&http.Client{Timeout: 60 * time.Second})),
		}, nil
	case policy.PolicyRoot:
		return []types.Check{
			&policy.HasLicenseCheck{},
			policy.NewHasUniqueTagCheck(cfg.DockerConfig),
			&policy.MaxLayersCheck{},
			&policy.HasNoProhibitedPackagesCheck{},
			&policy.HasRequiredLabelsCheck{},
			&policy.HasModifiedFilesCheck{},
			policy.NewBasedOnUbiCheck(pyxis.NewPyxisClient(
				cfg.PyxisHost,
				cfg.PyxisAPIToken,
				cfg.CertificationProjectID,
				&http.Client{Timeout: 60 * time.Second})),
		}, nil
	case policy.PolicyScratch:
		return []types.Check{
			&policy.HasLicenseCheck{},
			policy.NewHasUniqueTagCheck(cfg.DockerConfig),
			&policy.MaxLayersCheck{},
			&policy.HasRequiredLabelsCheck{},
			&policy.RunAsNonRootCheck{},
		}, nil
	}

	return nil, fmt.Errorf("provided container policy %s is unknown", p)
}
