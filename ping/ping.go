// Package ping provides facilities to measure ping and report the results.
package ping

import (
	"fmt"
	"log"
	"time"

	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
)

var pingGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "netmon",
		Subsystem: "ping",
		Name:      "avg_rtt_seconds",
		Help:      "Average RTT in seconds",
	},
	[]string{"address"},
)

func init() {
	prometheus.MustRegister(pingGauge)
}

// Ping the provided address and report metrics.
func Ping(address string) error {
	p, err := ping.NewPinger(address)
	if err != nil {
		return fmt.Errorf("ping: failed to create pinger: %w", err)
	}

	p.Count = 3
	p.Timeout = 20 * time.Second
	err = p.Run()
	if err != nil {
		return fmt.Errorf("ping: failed to run pinger: %w", err)
	}

	stats := p.Statistics()

	log.Printf("ping for %s: %dms\n", address, stats.AvgRtt.Milliseconds())
	pingGauge.WithLabelValues(address).Set(stats.AvgRtt.Seconds())
	return nil
}
