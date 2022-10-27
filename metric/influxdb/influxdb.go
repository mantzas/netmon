// Package influxdb is responsible for reporting our measurements to InfluxDB.
package influxdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-ping/ping"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/showwin/speedtest-go/speedtest"
)

// Config definition.
type Config struct {
	URL    string
	Token  string
	Org    string
	Bucket string
}

// Metric definition.
type Metric struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
}

// New constructor.
func New(ctx context.Context, cfg Config) (*Metric, error) {
	client := influxdb2.NewClient(cfg.URL, cfg.Token)

	ok, err := client.Ping(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("failed to ping metric server")
	}

	return &Metric{client: client, writeAPI: client.WriteAPIBlocking(cfg.Org, cfg.Bucket)}, nil
}

// ReportPing takes the result of a ping operation and writes a measurement.
func (mc *Metric) ReportPing(ctx context.Context, stats *ping.Statistics) error {
	p := influxdb2.NewPoint(
		"ping",
		map[string]string{
			"address": stats.Addr,
		},
		map[string]interface{}{
			"min_rtt":    stats.MinRtt.Seconds(),
			"max_rtt":    stats.MaxRtt.Seconds(),
			"avg_rtt":    stats.AvgRtt.Seconds(),
			"stddev_rtt": stats.StdDevRtt.Seconds(),
		},
		time.Now())

	err := mc.writeAPI.WritePoint(ctx, p)
	if err != nil {
		return fmt.Errorf("failed to write ping metric: %w", err)
	}

	return nil
}

// ReportSpeed takes the result of a speedtest operation and writes a measurement.
func (mc *Metric) ReportSpeed(ctx context.Context, srv *speedtest.Server) error {
	p := influxdb2.NewPoint(
		"speedtest",
		map[string]string{
			"server": fmt.Sprintf("%s - %s", srv.ID, srv.Sponsor),
		},
		map[string]interface{}{
			"latency": srv.Latency.Seconds(),
			"dl":      srv.DLSpeed,
			"ul":      srv.ULSpeed,
		},
		time.Now())

	err := mc.writeAPI.WritePoint(ctx, p)
	if err != nil {
		return fmt.Errorf("failed to write ping metric: %w", err)
	}

	return nil
}

// Close the underlying client.
func (mc *Metric) Close() {
	mc.client.Close()
}
