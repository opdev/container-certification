package flags

import (
	"fmt"

	"github.com/opdev/container-certification/internal/defaults"
	"github.com/spf13/pflag"
)

func BindFlagDockerConfigFilePath(f *pflag.FlagSet) {
	f.StringP(
		"docker-config",
		"d",
		"",
		"Path to docker config.json file. This value is optional for publicly accessible images.\n"+
			"However, it is strongly encouraged for public Docker Hub images,\n"+
			"due to the rate limit imposed for unauthenticated requests. (env: PFLT_DOCKERCONFIG)",
	)
}

func BindFlagPyxisAPIToken(f *pflag.FlagSet) {
	f.String("pyxis-api-token", "", "API token for Pyxis authentication (env: PFLT_PYXIS_API_TOKEN)")
}

func BindFlagPyxisEnv(f *pflag.FlagSet) {
	f.String("pyxis-env", defaults.DefaultPyxisEnv, "Env to use for Pyxis submissions.")
}

func BindFlagPyxisHost(f *pflag.FlagSet) {
	f.String("pyxis-host", "", fmt.Sprintf("Host to use for Pyxis submissions. This will override Pyxis Env. Only set this if you know what you are doing.\n"+
		"If you do set it, it should include just the host, and the URI path. (env: PFLT_PYXIS_HOST)"))
}
