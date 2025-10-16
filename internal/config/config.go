package config

import (
	"github.com/kelseyhightower/envconfig"
)

var singleConfig *Config = nil

type Config struct {
	Service *svcConfig
}

type svcConfig struct {
	Address  string `envconfig:"DCM_ADDRESS" default:":8082"`
	BaseUrl  string `envconfig:"DCM_BASE_URL" default:"https://localhost:8082"`
	LogLevel string `envconfig:"DCM_LOG_LEVEL" default:"info"`
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
