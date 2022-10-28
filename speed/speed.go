// Package speed provides facilities to speedtest and report the results.
package speed

import (
	"context"
	"fmt"
	"log"

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

// Test runs a speed test for the predefined server ids and report metrics.
func Test(ctx context.Context, serverIDs []int) error {
	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		return err
	}

	serverList, err := speedtest.FetchServerListContext(ctx, user)
	if err != nil {
		return err
	}

	targets, err := serverList.FindServer(serverIDs)
	if err != nil {
		return err
	}

	for _, target := range targets {
		serverName := fmt.Sprintf("%s - %s", target.ID, target.Sponsor)

		err := target.PingTestContext(ctx)
		if err != nil {
			return fmt.Errorf("speedtest: failed pint test: %w", err)
		}
		latencyGauge.WithLabelValues(serverName).Set(target.Latency.Seconds())

		err = target.DownloadTestContext(ctx, false)
		if err != nil {
			return fmt.Errorf("speedtest: failed download test: %w", err)
		}

		speedGauge.WithLabelValues(serverName, "dl").Set(target.DLSpeed)

		err = target.UploadTestContext(ctx, false)
		if err != nil {
			return fmt.Errorf("speedtest: failed upload test: %v", err)
		}

		speedGauge.WithLabelValues(serverName, "ul").Set(target.ULSpeed)

		log.Printf("speedtest for host: %s, latency: %s, dl: %f, ul: %f\n", serverName, target.Latency, target.DLSpeed, target.ULSpeed)
	}
	return nil
}
