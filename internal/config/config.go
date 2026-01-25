package config

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ProviderConfig *ProviderConfig
}

type ProviderConfig struct {
	Address string `envconfig:"SERVER_ADDRESS" default:"0.0.0.0:8080"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
