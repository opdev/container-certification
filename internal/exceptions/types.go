package exceptions

import (
	"context"

	"github.com/opdev/container-certification/internal/pyxis"
)

// Note(Jose) Borrowed directly from the internal/lib

// PyxisClient defines pyxis API interactions that are relevant to check executions in cmd.
type PyxisClient interface {
	FindImagesByDigest(ctx context.Context, digests []string) ([]pyxis.CertImage, error)
	GetProject(context.Context) (*pyxis.CertProject, error)
	SubmitResults(context.Context, *pyxis.CertificationInput) (*pyxis.CertificationResults, error)
}
