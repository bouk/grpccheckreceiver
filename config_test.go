// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package grpccheckreceiver // import "bou.ke/grpccheckreceiver"

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"bou.ke/grpccheckreceiver/internal/metadata"
)

func TestValidate(t *testing.T) {
	testCases := []struct {
		desc        string
		cfg         *Config
		expectedErr error
	}{
		{
			desc: "missing targets",
			cfg: &Config{
				Targets:          []*targetConfig{},
				ControllerConfig: scraperhelper.NewDefaultControllerConfig(),
			},
			expectedErr: errMissingTargets,
		},
		{
			desc: "missing endpoint",
			cfg: &Config{
				Targets: []*targetConfig{
					{
						ClientConfig: configgrpc.ClientConfig{},
					},
				},
				ControllerConfig: scraperhelper.NewDefaultControllerConfig(),
			},
			expectedErr: errMissingEndpoint,
		},
		{
			desc: "valid config",
			cfg: &Config{
				Targets: []*targetConfig{
					{
						ClientConfig: configgrpc.ClientConfig{
							Endpoint: "localhost:50051",
						},
					},
					{
						ClientConfig: configgrpc.ClientConfig{
							Endpoint: "example.com:443",
						},
						Service: "my.service",
					},
				},
				ControllerConfig: scraperhelper.NewDefaultControllerConfig(),
			},
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			actualErr := tc.cfg.Validate()
			if tc.expectedErr != nil {
				require.EqualError(t, actualErr, tc.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	tests := []struct {
		id       component.ID
		expected component.Config
	}{
		{
			id: component.NewID(metadata.Type),
			expected: &Config{
				ControllerConfig: scraperhelper.ControllerConfig{
					CollectionInterval: 1 * time.Minute,
					InitialDelay:       1 * time.Second,
				},
				Targets: []*targetConfig{
					{
						ClientConfig: configgrpc.ClientConfig{
							Endpoint: "localhost:50051",
							TLSSetting: configtls.ClientConfig{
								Insecure: true,
							},
						},
					},
					{
						ClientConfig: configgrpc.ClientConfig{
							Endpoint: "example.com:443",
						},
						Service: "my.service",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			tempCfg := &Config{}
			receivers, err := cm.Sub("receivers")
			require.NoError(t, err)
			receiver, err := receivers.Sub(tt.id.String())
			require.NoError(t, err)
			require.NoError(t, receiver.Unmarshal(tempCfg))
			assert.Equal(t, tt.expected.(*Config).ControllerConfig, tempCfg.ControllerConfig)
			assert.Len(t, tempCfg.Targets, len(tt.expected.(*Config).Targets))
		})
	}
}
