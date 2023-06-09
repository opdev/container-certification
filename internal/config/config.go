package config

import (
	"github.com/redhat-openshift-ecosystem/openshift-preflight/artifacts"
	spfviper "github.com/spf13/viper"
)

func Initialize(viper *spfviper.Viper) {
	// set up ENV var support
	viper.SetEnvPrefix("knex")
	viper.AutomaticEnv()

	// Set up logging config defaults
	viper.SetDefault("logfile", DefaultLogFile)
	viper.SetDefault("loglevel", DefaultLogLevel)
	viper.SetDefault("artifacts", artifacts.DefaultArtifactsDir)

	// Set up cluster defaults
	viper.SetDefault("namespace", DefaultNamespace)
	viper.SetDefault("serviceaccount", DefaultServiceAccount)

	// Set up scorecard wait time default
	viper.SetDefault("scorecard_wait_time", DefaultScorecardWaitTime)
}

var (
	DefaultLogFile           = "preflight.log"
	DefaultLogLevel          = "info"
	DefaultNamespace         = "default"
	DefaultServiceAccount    = "default"
	DefaultScorecardWaitTime = "240"
)
