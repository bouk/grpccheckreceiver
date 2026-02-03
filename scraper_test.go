// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package grpccheckreceiver // import "bou.ke/grpccheckreceiver"

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/pmetrictest"
	"bou.ke/grpccheckreceiver/internal/metadata"
)

type mockHealthServer struct {
	server   *grpc.Server
	listener net.Listener
	health   *health.Server
}

func newMockHealthServer(t *testing.T) *mockHealthServer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(s, hs)

	go func() {
		_ = s.Serve(lis)
	}()

	return &mockHealthServer{
		server:   s,
		listener: lis,
		health:   hs,
	}
}

func (m *mockHealthServer) addr() string {
	return m.listener.Addr().String()
}

func (m *mockHealthServer) shutdown() {
	m.server.GracefulStop()
}

func TestScraper(t *testing.T) {
	srv := newMockHealthServer(t)
	srv.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	defer srv.shutdown()

	expectedFile := filepath.Join("testdata", "expected_metrics", "expected.yaml")
	expectedMetrics, err := golden.ReadMetrics(expectedFile)
	require.NoError(t, err)

	conn, err := grpc.NewClient(srv.addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	cfg := &Config{
		MetricsBuilderConfig: metadata.DefaultMetricsBuilderConfig(),
		Targets: []*targetConfig{
			{
				ClientConfig: configgrpc.ClientConfig{
					Endpoint: srv.addr(),
				},
			},
		},
	}

	settings := receivertest.NewNopSettings(metadata.Type)
	scraper := newScraper(cfg, settings)
	scraper.conns = []*grpc.ClientConn{conn}

	actualMetrics, err := scraper.scrape(t.Context())
	require.NoError(t, err)

	require.NoError(t,
		pmetrictest.CompareMetrics(
			expectedMetrics,
			actualMetrics,
			pmetrictest.IgnoreMetricValues("grpccheck.duration"),
			pmetrictest.IgnoreTimestamp(),
			pmetrictest.IgnoreStartTimestamp(),
			pmetrictest.IgnoreMetricAttributeValue("grpc.endpoint"),
		),
	)
}

func TestScraperNotServing(t *testing.T) {
	srv := newMockHealthServer(t)
	srv.health.SetServingStatus("my.service", healthpb.HealthCheckResponse_NOT_SERVING)
	defer srv.shutdown()

	expectedFile := filepath.Join("testdata", "expected_metrics", "expected_not_serving.yaml")
	expectedMetrics, err := golden.ReadMetrics(expectedFile)
	require.NoError(t, err)

	conn, err := grpc.NewClient(srv.addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	cfg := &Config{
		MetricsBuilderConfig: metadata.DefaultMetricsBuilderConfig(),
		Targets: []*targetConfig{
			{
				ClientConfig: configgrpc.ClientConfig{
					Endpoint: srv.addr(),
				},
				Service: "my.service",
			},
		},
	}

	settings := receivertest.NewNopSettings(metadata.Type)
	scraper := newScraper(cfg, settings)
	scraper.conns = []*grpc.ClientConn{conn}

	actualMetrics, err := scraper.scrape(t.Context())
	require.NoError(t, err)

	require.NoError(t,
		pmetrictest.CompareMetrics(
			expectedMetrics,
			actualMetrics,
			pmetrictest.IgnoreMetricValues("grpccheck.duration"),
			pmetrictest.IgnoreTimestamp(),
			pmetrictest.IgnoreStartTimestamp(),
			pmetrictest.IgnoreMetricAttributeValue("grpc.endpoint"),
		),
	)
}

func TestScraperConnectionError(t *testing.T) {
	// Create a connection to a non-existent endpoint
	conn, err := grpc.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	cfg := &Config{
		MetricsBuilderConfig: metadata.DefaultMetricsBuilderConfig(),
		Targets: []*targetConfig{
			{
				ClientConfig: configgrpc.ClientConfig{
					Endpoint: "127.0.0.1:1",
				},
			},
		},
	}

	settings := receivertest.NewNopSettings(metadata.Type)
	scraper := newScraper(cfg, settings)
	scraper.conns = []*grpc.ClientConn{conn}

	actualMetrics, err := scraper.scrape(t.Context())
	require.NoError(t, err)

	// Should have error and status metrics recorded
	require.Greater(t, actualMetrics.ResourceMetrics().Len(), 0)
}
