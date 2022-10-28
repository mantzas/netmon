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
		Name:      "rtt_seconds",
		Help:      "RTT in seconds by type",
	},
	[]string{"address", "type"},
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
	pingGauge.WithLabelValues(address, "avg").Set(stats.AvgRtt.Seconds())
	pingGauge.WithLabelValues(address, "min").Set(stats.MinRtt.Seconds())
	pingGauge.WithLabelValues(address, "max").Set(stats.MaxRtt.Seconds())
	pingGauge.WithLabelValues(address, "stddev").Set(stats.StdDevRtt.Seconds())
	return nil
}
