package defaults

// TODO(Jose): Need to evaluate these defaults to make sure they all need to
// live in this package, or need to be shared.
var (
	DefaultCertImageFilename    = "cert-image.json"
	DefaultRPMManifestFilename  = "rpm-manifest.json"
	DefaultTestResultsFilename  = "results.json"
	DefaultArtifactsTarFileName = "artifacts.tar"
	DefaultPyxisHost            = "catalog.redhat.com/api/containers"
	DefaultPyxisEnv             = "prod"
	SystemdDir                  = "/etc/systemd/system"
)
