// Package netmon contains the network monitoring related code.
package netmon

import (
	"context"
	"fmt"
	"log"
	"time"

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

// SpeedTest runs against the provided servers. We can run either a full test or just a ping.
func SpeedTest(ctx context.Context, serverIDs []int, pingOnly bool) error {
	now := time.Now()
	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		return err
	}

	servers, err := speedtest.FetchServerListContext(ctx, user)
	if err != nil {
		return err
	}

	if !pingOnly {
		servers, err = servers.FindServer(serverIDs)
		if err != nil {
			return err
		}
	}

	for _, server := range servers {
		serverName := fmt.Sprintf("%s - %s", server.ID, server.Sponsor)

		err := server.PingTestContext(ctx)
		if err != nil {
			return fmt.Errorf("speedtest: failed pint test: %w", err)
		}
		latencyGauge.WithLabelValues(serverName).Set(server.Latency.Seconds())

		if pingOnly {
			log.Printf("speedtest for host: %s, latency: %s\n", serverName, server.Latency)
			continue
		}

		err = server.DownloadTestContext(ctx, false)
		if err != nil {
			return fmt.Errorf("speedtest: failed download test: %w", err)
		}

		speedGauge.WithLabelValues(serverName, "dl").Set(server.DLSpeed)

		err = server.UploadTestContext(ctx, false)
		if err != nil {
			return fmt.Errorf("speedtest: failed upload test: %v", err)
		}

		speedGauge.WithLabelValues(serverName, "ul").Set(server.ULSpeed)

		log.Printf("speedtest for host: %s, latency: %s, dl: %f, ul: %f\n", serverName, server.Latency, server.DLSpeed, server.ULSpeed)
	}
	log.Printf("speedtest duration: %v, ping only: %t", time.Since(now), pingOnly)
	return nil
}
