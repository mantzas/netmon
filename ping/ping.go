// Package ping provides facilities to measure ping and report the results.
package ping

import (
	"context"
	"sync"
	"time"

	"github.com/go-ping/ping"
	"github.com/mantzas/netmon/log"
)

// Config definition.
type Config struct {
	Addresses []string
	Interval  time.Duration
}

// MetricAPI definition.
type MetricAPI interface {
	ReportPing(context.Context, *ping.Statistics) error
}

// Monitor definition.
type Monitor struct {
	logger    log.Logger
	cfg       Config
	metricAPI MetricAPI
}

// New constructs a new ping monitor.
func New(metricAPI MetricAPI, logger log.Logger, cfg Config) (*Monitor, error) {
	return &Monitor{metricAPI: metricAPI, logger: logger, cfg: cfg}, nil
}

// Monitor starts the measurement.
func (pm *Monitor) Monitor(ctx context.Context) {
	tc := time.NewTicker(pm.cfg.Interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tc.C:
			tc.Stop()
			wg := sync.WaitGroup{}
			wg.Add(len(pm.cfg.Addresses))
			for _, address := range pm.cfg.Addresses {
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

func (pm *Monitor) measure(ctx context.Context, address string) {
	p, err := ping.NewPinger(address)
	if err != nil {
		pm.logger.Printf("ping: failed to create pinger: %v\n", err)
		return
	}

	p.Count = 3
	p.Timeout = 20 * time.Second
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
