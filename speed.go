// Package netmon contains the network monitoring related code.
package netmon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

	span := trace.SpanFromContext(ctx)
	tracer := span.TracerProvider().Tracer("netmon")

	servers, err := fetchServers(ctx, tracer)
	if err != nil {
		return nil, err
	}

	results := make([]PingResult, 0, len(servers))

	for _, server := range servers {
		results = append(results, pingTest(ctx, tracer, server))
	}

	slog.Debug("ping measurement", "duration", time.Since(now))
	return results, nil
}

func fetchServers(ctx context.Context, tracer trace.Tracer) ([]*speedtest.Server, error) {
	ctx, sp := tracer.Start(ctx, "FetchServers")
	defer sp.End()

	servers, err := speedtest.FetchServerListContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	return servers, nil
}

func pingTest(ctx context.Context, tracer trace.Tracer, server *speedtest.Server) PingResult {
	ctx, sp := tracer.Start(ctx, "PingTestContext")
	defer sp.End()
	sp.SetAttributes(attribute.String("server_id", server.ID))
	sp.SetAttributes(attribute.String("server", server.Sponsor))

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

	return result
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

	span := trace.SpanFromContext(ctx)
	tracer := span.TracerProvider().Tracer("netmon")

	results := make([]SpeedResult, 0, len(serverIDs))

	for _, serverID := range serverIDs {
		result := SpeedResult{
			ServerID: serverID,
		}

		server, err := fetchServerByID(ctx, tracer, serverID)
		if err != nil {
			result.Err = fmt.Errorf("failed to fetch server: %w", err)
			results = append(results, result)
			continue
		}

		result.Server = server.Sponsor

		serverName := fmt.Sprintf("%s - %s", server.ID, server.Sponsor)

		err = downloadTest(ctx, tracer, server)
		if err != nil {
			result.Err = fmt.Errorf("failed download test: %w", err)
			results = append(results, result)
			continue
		}

		result.DL = server.DLSpeed
		speedGauge.WithLabelValues(serverName, "dl").Set(server.DLSpeed)

		err = uploadTest(ctx, tracer, server)
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

func fetchServerByID(ctx context.Context, tracer trace.Tracer, serverID string) (*speedtest.Server, error) {
	_, sp := tracer.Start(ctx, "FetchServerByID")
	defer sp.End()

	server, err := speedtest.FetchServerByID(serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server: %w", err)
	}

	return server, nil
}

func downloadTest(ctx context.Context, tracer trace.Tracer, server *speedtest.Server) error {
	_, sp := tracer.Start(ctx, "DownloadTestContext")
	defer sp.End()

	return server.DownloadTestContext(ctx)
}

func uploadTest(ctx context.Context, tracer trace.Tracer, server *speedtest.Server) error {
	_, sp := tracer.Start(ctx, "UploadTestContext")
	defer sp.End()

	return server.UploadTestContext(ctx)
}
