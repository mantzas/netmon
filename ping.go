package netmon

import (
	"context"
	"sync"
	"time"

	"github.com/go-ping/ping"
)

type PingConfig struct {
	Addresses []string
	Interval  time.Duration
}

type PingMetricAPI interface {
	ReportPing(context.Context, *ping.Statistics) error
}

type PingMonitor struct {
	logger    Logger
	cfg       PingConfig
	metricAPI PingMetricAPI
}

func NewPingMonitor(metricAPI PingMetricAPI, logger Logger, cfg PingConfig) (*PingMonitor, error) {
	return &PingMonitor{metricAPI: metricAPI, logger: logger, cfg: cfg}, nil
}

func (pm *PingMonitor) Monitor(ctx context.Context) {
	tc := time.NewTicker(pm.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tc.C:
			tc.Stop()
			wg := sync.WaitGroup{}
			for _, address := range pm.cfg.Addresses {
				wg.Add(1)
				go func(addr string) {
					pm.measure(ctx, addr)
					wg.Done()
				}(address)
			}
			wg.Wait()
			tc.Reset(pm.cfg.Interval)
		}
	}
}

func (pm *PingMonitor) measure(ctx context.Context, address string) {
	p, err := ping.NewPinger(address)
	if err != nil {
		pm.logger.Printf("ping: failed to create pinger: %v\n", err)
		return
	}

	p.Count = 10
	err = p.Run()
	if err != nil {
		pm.logger.Printf("ping: failed to run pinger: %v\n", err)
		return
	}

	stats := p.Statistics()

	pm.logger.Printf("ping for %s: %dms\n", address, stats.AvgRtt.Milliseconds())

	err = pm.metricAPI.ReportPing(ctx, stats)
	if err != nil {
		pm.logger.Printf("ping: failed report metrics: %v\n", err)
		return
	}
}
