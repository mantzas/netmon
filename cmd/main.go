package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mantzas/netmon"
)

func main() {
	logger := &netmon.Log{}

	cfg, err := configFromEnv()
	if err != nil {
		logger.Fatalln(err)
	}

	ctx, cnl := context.WithCancel(context.Background())

	client, err := netmon.NewMetricClient(ctx, cfg.influxdb)
	if err != nil {
		logger.Fatalln(err)
	}
	defer client.Close()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, os.Interrupt, syscall.SIGTERM)

	pm, err := netmon.NewPingMonitor(client, logger, cfg.ping)
	if err != nil {
		logger.Fatalln(err)
	}

	sm, err := netmon.NewSpeedMonitor(ctx, client, logger, cfg.speed)
	if err != nil {
		logger.Fatalln(err)
	}

	wg := sync.WaitGroup{}

	wg.Add(1)

	go func() {
		pm.Monitor(ctx)
		wg.Done()
	}()

	wg.Add(1)

	go func() {
		sm.Monitor(ctx)
		wg.Done()
	}()

	for {
		select {
		case sig := <-chSignal:
			logger.Printf("signal %v received\n", sig)
			cnl()
		case <-ctx.Done():
			wg.Wait()
			os.Exit(0)
		}
	}
}

type config struct {
	influxdb netmon.InfluxDBConfig
	ping     netmon.PingConfig
	speed    netmon.SpeedConfig
}

func configFromEnv() (config, error) {
	// speedtest server, ids comma separated

	var err error
	cfg := config{}

	cfg.influxdb, err = getInfluxDBConfig()
	if err != nil {
		return config{}, err
	}

	cfg.ping, err = getPingConfig()
	if err != nil {
		return config{}, err
	}

	cfg.speed, err = getSpeedConfig()
	if err != nil {
		return config{}, err
	}

	return cfg, nil
}

func getInfluxDBConfig() (netmon.InfluxDBConfig, error) {
	var err error
	cfg := netmon.InfluxDBConfig{}
	cfg.URL, err = getEnv("INFLUXDB_URL")
	if err != nil {
		return netmon.InfluxDBConfig{}, err
	}

	cfg.Token, err = getEnv("INFLUXDB_TOKEN")
	if err != nil {
		return netmon.InfluxDBConfig{}, err
	}

	cfg.Org, err = getEnv("INFLUXDB_ORG")
	if err != nil {
		return netmon.InfluxDBConfig{}, err
	}

	cfg.Bucket, err = getEnv("INFLUXDB_BUCKET")
	if err != nil {
		return netmon.InfluxDBConfig{}, err
	}

	return cfg, nil
}

func getPingConfig() (netmon.PingConfig, error) {
	var err error
	cfg := netmon.PingConfig{}

	url, err := getEnv("PING_ADDRESSES")
	if err != nil {
		return netmon.PingConfig{}, err
	}

	cfg.Addresses = strings.Split(url, ",")

	secVal, err := getEnv("PING_INTERVAL_SECONDS")
	if err != nil {
		return netmon.PingConfig{}, err
	}

	seconds, err := strconv.Atoi(secVal)
	if err != nil {
		return netmon.PingConfig{}, fmt.Errorf("failed to convert ping interval: %v", err)
	}

	cfg.Interval = time.Duration(seconds) * time.Second

	return cfg, nil
}

func getSpeedConfig() (netmon.SpeedConfig, error) {
	var err error
	cfg := netmon.SpeedConfig{}

	ids, err := getEnv("SPEED_SERVER_IDS")
	if err != nil {
		return netmon.SpeedConfig{}, err
	}

	for _, id := range strings.Split(ids, ",") {

		serverID, err := strconv.Atoi(id)
		if err != nil {
			return netmon.SpeedConfig{}, fmt.Errorf("failed to convert server id [%s]: %v", id, err)
		}

		cfg.ServerIDs = append(cfg.ServerIDs, serverID)
	}

	secVal, err := getEnv("SPEED_INTERVAL_SECONDS")
	if err != nil {
		return netmon.SpeedConfig{}, err
	}

	seconds, err := strconv.Atoi(secVal)
	if err != nil {
		return netmon.SpeedConfig{}, fmt.Errorf("failed to convert speed interval: %v", err)
	}

	cfg.Interval = time.Duration(seconds) * time.Second

	return cfg, nil
}

func getEnv(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("env var %s does not exist", key)
	}
	if value == "" {
		return "", fmt.Errorf("env var %s does not exist", key)
	}
	return value, nil
}
