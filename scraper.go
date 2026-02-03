// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package grpccheckreceiver // import "bou.ke/grpccheckreceiver"

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"

	"bou.ke/grpccheckreceiver/internal/metadata"
)

var errClientNotInit = errors.New("client not initialized")

type grpccheckScraper struct {
	cfg      *Config
	settings component.TelemetrySettings
	mb       *metadata.MetricsBuilder
	conns    []*grpc.ClientConn
}

func (s *grpccheckScraper) start(ctx context.Context, host component.Host) error {
	var err error
	for _, target := range s.cfg.Targets {
		conn, connErr := target.ToClientConn(ctx, host, s.settings)
		if connErr != nil {
			s.settings.Logger.Error("failed to create gRPC client connection",
				zap.String("endpoint", target.Endpoint),
				zap.Error(connErr))
			err = multierr.Append(err, connErr)
			s.conns = append(s.conns, nil)
			continue
		}
		s.conns = append(s.conns, conn)
	}
	return err
}

func (s *grpccheckScraper) shutdown(_ context.Context) error {
	var err error
	for _, conn := range s.conns {
		if conn != nil {
			if closeErr := conn.Close(); closeErr != nil {
				err = multierr.Append(err, closeErr)
			}
		}
	}
	return err
}

func (s *grpccheckScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	if len(s.conns) == 0 {
		return pmetric.NewMetrics(), errClientNotInit
	}

	var wg sync.WaitGroup
	var mux sync.Mutex

	for idx, conn := range s.conns {
		if conn == nil {
			continue
		}

		wg.Add(1)
		go func(targetIdx int, clientConn *grpc.ClientConn) {
			defer wg.Done()

			target := s.cfg.Targets[targetIdx]
			now := pcommon.NewTimestampFromTime(time.Now())

			client := healthpb.NewHealthClient(clientConn)

			var p peer.Peer
			start := time.Now()
			resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{
				Service: target.Service,
			}, grpc.Peer(&p))
			duration := time.Since(start).Milliseconds()

			mux.Lock()
			defer mux.Unlock()

			if err != nil {
				s.mb.RecordGrpccheckErrorDataPoint(now, int64(1), target.Endpoint, target.Service, err.Error())
				s.mb.RecordGrpccheckStatusDataPoint(now, int64(0), target.Endpoint, target.Service)
				return
			}

			s.mb.RecordGrpccheckDurationDataPoint(now, duration, target.Endpoint, target.Service)
			s.mb.RecordGrpccheckResponseSizeDataPoint(now, int64(proto.Size(resp)), target.Endpoint, target.Service)

			var statusValue int64
			if resp.GetStatus() == healthpb.HealthCheckResponse_SERVING {
				statusValue = 1
			}
			s.mb.RecordGrpccheckStatusDataPoint(now, statusValue, target.Endpoint, target.Service)

			s.recordTLSCertMetrics(now, target.Endpoint, &p)
		}(idx, conn)
	}

	wg.Wait()

	return s.mb.Emit(), nil
}

func (s *grpccheckScraper) recordTLSCertMetrics(now pcommon.Timestamp, endpoint string, p *peer.Peer) {
	if p.AuthInfo == nil {
		return
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return
	}

	recordTLSCert(s.mb, now, endpoint, &tlsInfo.State)
}

func recordTLSCert(mb *metadata.MetricsBuilder, now pcommon.Timestamp, endpoint string, state *tls.ConnectionState) {
	for _, cert := range state.PeerCertificates {
		issuer := cert.Issuer.String()
		cn := cert.Subject.CommonName

		sans := make([]any, 0, len(cert.DNSNames)+len(cert.IPAddresses))
		for _, dns := range cert.DNSNames {
			sans = append(sans, dns)
		}
		for _, ip := range cert.IPAddresses {
			sans = append(sans, ip.String())
		}

		remaining := time.Until(cert.NotAfter).Seconds()
		mb.RecordGrpccheckTLSCertRemainingDataPoint(now, int64(remaining), endpoint, issuer, cn, sans)
	}
}

func newScraper(cfg *Config, settings receiver.Settings) *grpccheckScraper {
	return &grpccheckScraper{
		cfg:      cfg,
		settings: settings.TelemetrySettings,
		mb:       metadata.NewMetricsBuilder(cfg.MetricsBuilderConfig, settings),
	}
}
