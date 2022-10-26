package netmon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

type SpeedConfig struct {
	ServerIDs []int
	Interval  time.Duration
}

type SpeedMetricAPI interface {
	ReportSpeed(context.Context, *speedtest.Server) error
}

type SpeedMonitor struct {
	metricAPI SpeedMetricAPI
	logger    Logger
	cfg       SpeedConfig
	targets   speedtest.Servers
}

func NewSpeedMonitor(ctx context.Context, metricAPI SpeedMetricAPI, logger Logger, cfg SpeedConfig) (*SpeedMonitor, error) {
	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		return nil, err
	}

	serverList, err := speedtest.FetchServerListContext(ctx, user)
	if err != nil {
		return nil, err
	}

	targets, err := serverList.FindServer(cfg.ServerIDs)
	if err != nil {
		return nil, err
	}
	return &SpeedMonitor{metricAPI: metricAPI, logger: logger, cfg: cfg, targets: targets}, nil
}

func (sm *SpeedMonitor) Monitor(ctx context.Context) {
	tc := time.NewTicker(sm.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tc.C:
			tc.Stop()
			wg := sync.WaitGroup{}

			for _, target := range sm.targets {
				wg.Add(1)
				go func(target *speedtest.Server) {
					sm.measure(ctx, target)
					wg.Done()
				}(target)
			}
			wg.Wait()
			tc.Reset(sm.cfg.Interval)
		}
	}
}

func (sm *SpeedMonitor) measure(ctx context.Context, srv *speedtest.Server) {
	serverName := fmt.Sprintf("%s - %s", srv.ID, srv.Sponsor)
	srv.PingTestContext(ctx)
	srv.DownloadTestContext(ctx, false)
	srv.UploadTestContext(ctx, false)

	sm.logger.Printf("speedtest for host: %s, latency: %s, dl: %f, ul: %f\n", serverName, srv.Latency, srv.DLSpeed, srv.ULSpeed)
	err := sm.metricAPI.ReportSpeed(ctx, srv)
	if err != nil {
		sm.logger.Printf("speedtest: failed report metrics: %v\n", err)
		return
	}
}
