package config

import (
	"github.com/kelseyhightower/envconfig"
)

var singleConfig *Config = nil

type Config struct {
	Service *svcConfig
}

type svcConfig struct {
	Address          string `envconfig:"DCM_ADDRESS" default:":8082"`
	BaseUrl          string `envconfig:"KUBEVIRT_PROVIDER_URL" default:"http://localhost:8082"`
	RegistryUrl      string `envconfig:"DCM_SERVICE_PROVIDER_URL" default:"http://localhost:8081"`
	RegistryEndpoint string `envconfig:"DCM_SERVICE_PROVIDER_ENDPOINT" default:"/providers"`
	LogLevel         string `envconfig:"DCM_LOG_LEVEL" default:"info"`
}

func New() (*Config, error) {
	if singleConfig == nil {
		singleConfig = new(Config)
		if err := envconfig.Process("", singleConfig); err != nil {
			return nil, err
		}
	}
	return singleConfig, nil
}
