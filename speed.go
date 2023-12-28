// Package netmon contains the network monitoring related code.
package netmon

import (
	"context"
	"fmt"
	"log/slog"
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

// PingResult contains the ping test result.
type PingResult struct {
	ServerID string        `json:"server_id"`
	Server   string        `json:"server"`
	Latency  time.Duration `json:"latency"`
	Err      error         `json:"error"`
}

// Ping runs a ping test against the provided servers.
func Ping(ctx context.Context) ([]PingResult, error) {
	now := time.Now()

	servers, err := speedtest.FetchServerListContext(ctx)
	if err != nil {
		return nil, err
	}

	var results []PingResult

	for _, server := range servers {
		result := PingResult{
			ServerID: server.ID,
			Server:   server.Sponsor,
		}

		err := server.PingTestContext(ctx, func(latency time.Duration) {
			result.Latency = latency
			latencyGauge.WithLabelValues(result.Server).Set(latency.Seconds())
		})
		if err != nil {
			result.Err = fmt.Errorf("ping: failed ping test on %s: %w", result.Server, err)
		}
		results = append(results, result)
	}
	slog.Debug("ping measurement", "duration", time.Since(now))
	return results, nil
}

// SpeedResult contains the speed test result.
type SpeedResult struct {
	ServerID string        `json:"server_id"`
	Server   string        `json:"server"`
	Latency  time.Duration `json:"latency"`
	DL       float64       `json:"dl"`
	UL       float64       `json:"ul"`
	Err      error         `json:"error"`
}

// Speed runs a speed test against the provided servers.
func Speed(ctx context.Context, serverIDs []string) []SpeedResult {
	now := time.Now()

	var results []SpeedResult

	for _, serverID := range serverIDs {
		result := SpeedResult{
			ServerID: serverID,
		}
		server, err := speedtest.FetchServerByID(serverID)
		if err != nil {
			result.Err = fmt.Errorf("failed to fetch server: %w", err)
			results = append(results, result)
			continue
		}

		result.Server = server.Sponsor

		serverName := fmt.Sprintf("%s - %s", server.ID, server.Sponsor)

		err = server.PingTestContext(ctx, func(latency time.Duration) {
			result.Latency = latency
			latencyGauge.WithLabelValues(serverName).Set(latency.Seconds())
		})
		if err != nil {
			result.Err = fmt.Errorf("failed ping test on %w", err)
			results = append(results, result)
			continue
		}

		err = server.DownloadTestContext(ctx)
		if err != nil {
			result.Err = fmt.Errorf("failed download test: %w", err)
			results = append(results, result)
			continue
		}

		result.DL = server.DLSpeed
		speedGauge.WithLabelValues(serverName, "dl").Set(server.DLSpeed)

		err = server.UploadTestContext(ctx)
		if err != nil {
			result.Err = fmt.Errorf("failed upload test: %w", err)
			results = append(results, result)
			continue
		}

		result.UL = server.ULSpeed
		speedGauge.WithLabelValues(serverName, "ul").Set(server.ULSpeed)
		results = append(results, result)

		slog.Debug("speed measurement", "server", serverName, "latency", server.Latency, "dl", server.DLSpeed,
			"ul", server.ULSpeed)
	}

	slog.Debug("speed measurement", "duration", time.Since(now))
	return results
}
