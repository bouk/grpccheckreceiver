// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package grpccheckreceiver // import "bou.ke/grpccheckreceiver"

import (
	"errors"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/scraper/scraperhelper"
	"go.uber.org/multierr"

	"bou.ke/grpccheckreceiver/internal/metadata"
)

var (
	errMissingTargets  = errors.New("no targets configured")
	errMissingEndpoint = errors.New("endpoint must be specified")
)

// Config defines the configuration for the gRPC check receiver.
type Config struct {
	scraperhelper.ControllerConfig `mapstructure:",squash"`
	metadata.MetricsBuilderConfig  `mapstructure:",squash"`
	Targets                        []*targetConfig `mapstructure:"targets"`

	// prevent unkeyed literal initialization
	_ struct{}
}

// targetConfig defines configuration for an individual gRPC health check target.
type targetConfig struct {
	configgrpc.ClientConfig `mapstructure:",squash"`
	Service                 string `mapstructure:"service"`
}

func (cfg *Config) Validate() error {
	var err error

	if len(cfg.Targets) == 0 {
		err = multierr.Append(err, errMissingTargets)
	}

	for _, target := range cfg.Targets {
		if target.Endpoint == "" {
			err = multierr.Append(err, errMissingEndpoint)
		}
	}

	return err
}
