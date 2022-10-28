// Package speed provides facilities to speedtest and report the results.
package speed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mantzas/netmon/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
)

var latencyGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "netmon",
		Subsystem: "speettest",
		Name:      "latency_seconds",
		Help:      "Latency in seconds",
	},
	[]string{"server"},
)

var speedGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "netmon",
		Subsystem: "speettest",
		Name:      "speed",
		Help:      "Up and download speed",
	},
	[]string{"server", "direction"},
)

func init() {
	prometheus.MustRegister(latencyGauge)
	prometheus.MustRegister(speedGauge)
}

// Config definition.
type Config struct {
	ServerIDs []int
	Interval  time.Duration
}

// Monitor definition.
type Monitor struct {
	logger  log.Logger
	cfg     Config
	targets speedtest.Servers
}

// New constructs a new speedtest monitor.
func New(ctx context.Context, logger log.Logger, cfg Config) (*Monitor, error) {
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
	return &Monitor{logger: logger, cfg: cfg, targets: targets}, nil
}

// Monitor starts the measurement.
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
	latencyGauge.WithLabelValues(serverName).Set(srv.Latency.Seconds())

	err = srv.DownloadTestContext(ctx, false)
	if err != nil {
		sm.logger.Printf("speedtest: failed download test: %v\n", err)
		return
	}

	speedGauge.WithLabelValues(serverName, "dl").Set(srv.DLSpeed)

	err = srv.UploadTestContext(ctx, false)
	if err != nil {
		sm.logger.Printf("speedtest: failed upload test: %v\n", err)
		return
	}

	speedGauge.WithLabelValues(serverName, "ul").Set(srv.ULSpeed)

	sm.logger.Printf("speedtest for host: %s, latency: %s, dl: %f, ul: %f\n", serverName, srv.Latency, srv.DLSpeed, srv.ULSpeed)
}
