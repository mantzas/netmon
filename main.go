package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-ping/ping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/showwin/speedtest-go/speedtest"
)

var (
	pingGauge         prometheus.GaugeVec
	speedLatencyGauge prometheus.GaugeVec
)

func init() {
	pingGauge = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netmon",
			Subsystem: "ping",
			Name:      "average_rtt_seconds",
			Help:      "Measures ping of a specific address",
		},
		[]string{"address"},
	)
	prometheus.MustRegister(pingGauge)

	speedLatencyGauge = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "netmon",
			Subsystem: "speedtest",
			Name:      "latency_seconds",
			Help:      "Measures latency of a specific server",
		},
		[]string{"server"},
	)
	prometheus.MustRegister(speedLatencyGauge)
}

func init() {
}

func main() {
	pingAddresses := []string{"1.1.1.1", "8.8.8.8"}

	for _, address := range pingAddresses {
		err := measurePing(address)
		if err != nil {
			log.Println(err)
		}
	}

	err := measureSpeed()
	if err != nil {
		log.Println(err)
	}
}

func measurePing(address string) error {
	p, err := ping.NewPinger(address)
	if err != nil {
		return err
	}

	p.Count = 10
	err = p.Run()
	if err != nil {
		return err
	}

	stats := p.Statistics()

	pingGauge.WithLabelValues(address).Set(float64(stats.AvgRtt.Seconds()))
	log.Printf("ping for %s: %dms\n", address, stats.AvgRtt.Milliseconds())
	return nil
}

func measureSpeed() error {
	ctx, cnl := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cnl()

	user, err := speedtest.FetchUserInfoContext(ctx)
	if err != nil {
		return err
	}

	fmt.Println(user)

	serverList, err := speedtest.FetchServerListContext(ctx, user)
	if err != nil {
		return err
	}

	fmt.Println(serverList)

	targets, err := serverList.FindServer([]int{5188})
	if err != nil {
		return err
	}

	fmt.Println(targets)

	for _, s := range targets {

		s.PingTestContext(ctx)

		speedLatencyGauge.WithLabelValues(s.Host).Set(s.Latency.Seconds())

		s.DownloadTestContext(ctx, false)
		s.UploadTestContext(ctx, false)

		fmt.Printf("Host: %s, Latency: %s, Download: %f, Upload: %f\n", s.Host, s.Latency, s.DLSpeed, s.ULSpeed)
	}

	return nil
}
