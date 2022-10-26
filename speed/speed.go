package speed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mantzas/netmon"
	"github.com/showwin/speedtest-go/speedtest"
)

// Config definition.
type Config struct {
	ServerIDs []int
	Interval  time.Duration
}

type MetricAPI interface {
	ReportSpeed(context.Context, *speedtest.Server) error
}

type Monitor struct {
	metricAPI MetricAPI
	logger    netmon.Logger
	cfg       Config
	targets   speedtest.Servers
}

func New(ctx context.Context, metricAPI MetricAPI, logger netmon.Logger, cfg Config) (*Monitor, error) {
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
	return &Monitor{metricAPI: metricAPI, logger: logger, cfg: cfg, targets: targets}, nil
}

func (sm *Monitor) Monitor(ctx context.Context) {
	tc := time.NewTicker(sm.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tc.C:
			tc.Stop()
			wg := sync.WaitGroup{}
			wg.Add(len(sm.targets))
			for _, target := range sm.targets {
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

func (sm *Monitor) measure(ctx context.Context, srv *speedtest.Server) {
	serverName := fmt.Sprintf("%s - %s", srv.ID, srv.Sponsor)

	err := srv.PingTestContext(ctx)
	if err != nil {
		sm.logger.Printf("speedtest: failed pint test: %v\n", err)
		return
	}

	err = srv.DownloadTestContext(ctx, false)
	if err != nil {
		sm.logger.Printf("speedtest: failed download test: %v\n", err)
		return
	}

	err = srv.UploadTestContext(ctx, false)
	if err != nil {
		sm.logger.Printf("speedtest: failed upload test: %v\n", err)
		return
	}

	sm.logger.Printf("speedtest for host: %s, latency: %s, dl: %f, ul: %f\n", serverName, srv.Latency, srv.DLSpeed, srv.ULSpeed)
	err = sm.metricAPI.ReportSpeed(ctx, srv)
	if err != nil {
		sm.logger.Printf("speedtest: failed report metrics: %v\n", err)
		return
	}
}
