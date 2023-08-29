package submit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/opdev/container-certification/internal/defaults"
	"github.com/opdev/container-certification/internal/policy"
	"github.com/opdev/container-certification/internal/pyxis"
	"github.com/opdev/knex/log"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/artifacts"

	"github.com/go-logr/logr"
)

// NOTE(Jose): this PyxisClient interface and the following NewPyxisClient call is used specifically for
// this package. Probably should be made private?

// PyxisClient defines pyxis API interactions that are relevant to check executions in cmd.
type PyxisClient interface {
	FindImagesByDigest(ctx context.Context, digests []string) ([]pyxis.CertImage, error)
	GetProject(context.Context) (*pyxis.CertProject, error)
	SubmitResults(context.Context, *pyxis.CertificationInput) (*pyxis.CertificationResults, error)
}

// NewPyxisClient initializes a pyxisClient with relevant information from cfg.
// If the the CertificationProjectID, PyxisAPIToken, or PyxisHost are empty, then nil is returned.
// Callers should treat a nil pyxis client as an indicator that pyxis calls should not be made.
func NewPyxisClient(_ context.Context, projectID, token, host string) PyxisClient {
	if projectID == "" || token == "" || host == "" {
		return nil
	}

	return pyxis.NewPyxisClient(
		host,
		token,
		projectID,
		&http.Client{Timeout: 60 * time.Second},
	)
}

// ContainerCertificationSubmitter submits container results to Pyxis, and implements
// a ResultSubmitter.
type ContainerCertificationSubmitter struct {
	CertificationProjectID string
	Pyxis                  PyxisClient
	DockerConfig           string
	PreflightLogFile       string
	// Note(Jose): Added PyxisEnv here so that URL building functiosn can be switched to methods on this type.
	PyxisEnv string
}

func (s *ContainerCertificationSubmitter) Submit(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("preparing results that will be submitted to Red Hat")

	// get the project info from pyxis
	certProject, err := s.Pyxis.GetProject(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve project: %w", err)
	}

	// Ensure that a certProject was returned. In theory we would expect pyxis
	// to throw an error if no project is returned, but in the event that it doesn't
	// we need to confirm before we proceed in order to prevent a runtime panic
	// setting the DockerConfigJSON below.
	if certProject == nil {
		return fmt.Errorf("no certification project was returned from pyxis")
	}

	logger.V(log.TRC).Info("certification project id", "project", certProject)

	// only read the dockerfile if the user provides a location for the file
	// at this point in the flow, if `cfg.DockerConfig` is empty we know the repo is public and can continue the submission flow
	if s.DockerConfig != "" {
		dockerConfigJSONBytes, err := os.ReadFile(s.DockerConfig)
		if err != nil {
			return fmt.Errorf("could not open file for submission: %s: %w",
				s.DockerConfig,
				err,
			)
		}

		certProject.Container.DockerConfigJSON = string(dockerConfigJSONBytes)
	}

	// the below code is for the edge case where a partner has a DockerConfig in pyxis, but does not send one to preflight.
	// when we call pyxis's GetProject API, we get back the DockerConfig as a PGP encrypted string and not JSON,
	// if we were to send what pyixs just sent us in a update call, pyxis would throw a validation error saying it's not valid json
	// the below code aims to set the DockerConfigJSON to an empty string, and since this field is `omitempty` when we marshall it
	// we will not get a validation error
	if s.DockerConfig == "" {
		certProject.Container.DockerConfigJSON = ""
	}

	// no longer set DockerConfigJSON for registries which Red Hat hosts, this prevents the user from sending an invalid
	// docker file that systems like clair and registry-proxy cannot use to pull the image
	if certProject.Container.HostedRegistry {
		certProject.Container.DockerConfigJSON = ""
	}

	// We need to get the artifact writer to know where our artifacts were written. We also need the
	// Filesystem Writer here to make sure we can get the configured path.
	// TODO: This needs to be rethought. Submission is not currently in scope for library implementations
	// but the current implementation of this makes it impossible because the MapWriter would obviously
	// not work here.
	artifactWriter, ok := artifacts.WriterFromContext(ctx).(*artifacts.FilesystemWriter)
	if artifactWriter == nil || !ok {
		return errors.New("the artifact writer was either missing or was not supported, so results cannot be submitted")
	}

	certImage, err := os.Open(path.Join(artifactWriter.Path(), defaults.DefaultCertImageFilename))
	if err != nil {
		return fmt.Errorf("could not open file for submission: %s: %w",
			defaults.DefaultCertImageFilename,
			err,
		)
	}
	defer certImage.Close()

	preflightResults, err := os.Open(path.Join(artifactWriter.Path(), defaults.DefaultTestResultsFilename))
	if err != nil {
		return fmt.Errorf(
			"could not open file for submission: %s: %w",
			defaults.DefaultTestResultsFilename,
			err,
		)
	}
	defer preflightResults.Close()

	logfile, err := os.Open(s.PreflightLogFile)
	if err != nil {
		return fmt.Errorf(
			"could not open file for submission: %s: %w",
			s.PreflightLogFile,
			err,
		)
	}
	defer logfile.Close()

	options := []pyxis.CertificationInputOption{
		pyxis.WithCertImage(certImage),
		pyxis.WithPreflightResults(preflightResults),
		pyxis.WithArtifact(logfile, filepath.Base(s.PreflightLogFile)),
	}

	pol := policy.PolicyContainer

	if certProject.ScratchProject() {
		pol = policy.PolicyScratch
	}

	// only read the rpm manifest file off of disk if the policy executed is not scratch
	// scratch images do not have rpm manifests, the rpm-manifest.json file is not written to disk by the engine during execution
	if pol != policy.PolicyScratch {
		rpmManifest, err := os.Open(path.Join(artifactWriter.Path(), defaults.DefaultRPMManifestFilename))
		if err != nil {
			return fmt.Errorf(
				"could not open file for submission: %s: %w",
				defaults.DefaultRPMManifestFilename,
				err,
			)
		}
		defer rpmManifest.Close()

		options = append(options, pyxis.WithRPMManifest(rpmManifest))
	}

	submission, err := pyxis.NewCertificationInput(ctx, certProject, options...)
	if err != nil {
		return fmt.Errorf("unable to finalize data that would be sent to pyxis: %w", err)
	}

	certResults, err := s.Pyxis.SubmitResults(ctx, submission)
	if err != nil {
		return fmt.Errorf("could not submit to pyxis: %w", err)
	}

	logger.Info("Test results have been submitted to Red Hat.")
	logger.Info("These results will be reviewed by Red Hat for final certification.")
	logger.Info(fmt.Sprintf("The container's image id is: %s.", certResults.CertImage.ID))
	logger.Info(fmt.Sprintf("Please check %s to view scan results.", s.BuildScanResultsURL(s.CertificationProjectID, certResults.CertImage.ID)))
	logger.Info(fmt.Sprintf("Please check %s to monitor the progress.", s.BuildOverviewURL(s.CertificationProjectID)))

	return nil
}

func (s *ContainerCertificationSubmitter) BuildConnectURL(projectID string) string {
	connectURL := fmt.Sprintf("https://connect.redhat.com/projects/%s", projectID)

	pyxisEnv := s.PyxisEnv
	if len(pyxisEnv) > 0 && pyxisEnv != "prod" {
		connectURL = fmt.Sprintf("https://connect.%s.redhat.com/projects/%s", s.PyxisEnv, projectID)
	}

	return connectURL
}

func (s *ContainerCertificationSubmitter) BuildOverviewURL(projectID string) string {
	return fmt.Sprintf("%s/overview", s.BuildConnectURL(projectID))
}

func (s *ContainerCertificationSubmitter) BuildScanResultsURL(projectID string, imageID string) string {
	return fmt.Sprintf("%s/images/%s/scan-results", s.BuildConnectURL(projectID), imageID)
}
