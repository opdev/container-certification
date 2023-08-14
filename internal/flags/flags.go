package flags

import (
	"fmt"
	"runtime"

	"github.com/opdev/container-certification/internal/defaults"
	"github.com/spf13/pflag"
)

const (
	KeyDockerConfig  = "docker-config"
	KeyPyxisAPIToken = "pyxis-api-token"
	KeyPyxisEnv      = "pyxis-env"
	KeyPyxisHost     = "pyxis-host"
	KeyPlatform      = "platform"
	KeyCertProjectID = "certification-project-id"
)

func BindFlagDockerConfigFilePath(f *pflag.FlagSet) {
	f.StringP(
		KeyDockerConfig,
		"d",
		"",
		"Path to docker config.json file. This value is optional for publicly accessible images.\n"+
			"However, it is strongly encouraged for public Docker Hub images,\n"+
			"due to the rate limit imposed for unauthenticated requests. (env: PFLT_DOCKERCONFIG)",
	)
}

func BindFlagPyxisAPIToken(f *pflag.FlagSet) {
	f.String(KeyPyxisAPIToken, "", "API token for Pyxis authentication (env: PFLT_PYXIS_API_TOKEN)")
}

func BindFlagPyxisEnv(f *pflag.FlagSet) {
	f.String(KeyPyxisEnv, defaults.DefaultPyxisEnv, "Env to use for Pyxis submissions.")
}

func BindFlagPyxisHost(f *pflag.FlagSet) {
	f.String(KeyPyxisHost, defaults.DefaultPyxisHost, fmt.Sprintf("Host to use for Pyxis submissions. This will override Pyxis Env. Only set this if you know what you are doing.\n"+
		"If you do set it, it should include just the host, and the URI path. (env: PFLT_PYXIS_HOST)"))
}

func BindFlagsImagePlatform(f *pflag.FlagSet) {
	f.String(KeyPlatform, runtime.GOARCH, "Architecture of image to pull. Defaults to current platform.")
}

func BindFlagCertificationProjectID(f *pflag.FlagSet) {
	f.String(
		KeyCertProjectID,
		"",
		fmt.Sprintf(
			"Certification Project ID from connect.redhat.com/projects/{certification-project-id}/overview\n"+
				"URL paramater. This value may differ from the PID on the overview page. (env: PFLT_CERTIFICATION_PROJECT_ID)",
		),
	)
}
