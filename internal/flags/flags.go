package flags

import "github.com/spf13/pflag"

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
