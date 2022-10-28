package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mantzas/netmon"
	"github.com/mantzas/netmon/ping"
	"github.com/mantzas/netmon/speed"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}

	log.Printf("starting monitoring: %+v", cfg)

	ctx, cnl := context.WithCancel(context.Background())
	defer cnl()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, os.Interrupt, syscall.SIGTERM)

	scheduler, err := netmon.NewScheduler(cfg.ping, cfg.speed, ping.Ping, speed.Test)
	if err != nil {
		return err
	}

	srv := createHTTPServer(cfg.httpPort)

	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.Printf("failed to run HTTP listener: %v", err)
		}
	}()

	go func() {
		scheduler.Schedule(ctx)
	}()

	for {
		select {
		case sig := <-chSignal:
			log.Printf("signal %v received\n", sig)
			err = srv.Close()
			if err != nil {
				return fmt.Errorf("failed to close HTTP server: %v", err)
			}
			cnl()
		case <-ctx.Done():
			return nil
		}
	}
}

func createHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       240 * time.Second,
		Handler:           http.TimeoutHandler(mux, 59*time.Second, ""),
	}
}

type config struct {
	httpPort int
	ping     netmon.PingConfig
	speed    netmon.SpeedConfig
}

func configFromEnv() (config, error) {
	cfg := config{}

	httpPort, err := getEnv("HTTP_PORT", "50001")
	if err != nil {
		return cfg, err
	}

	cfg.httpPort, err = strconv.Atoi(httpPort)
	if err != nil {
		return cfg, err
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

func getPingConfig() (netmon.PingConfig, error) {
	var err error
	cfg := netmon.PingConfig{}

	url, err := getEnv("PING_ADDRESSES", "1.1.1.1,8.8.8.8")
	if err != nil {
		return netmon.PingConfig{}, err
	}

	cfg.Addresses = strings.Split(url, ",")

	secVal, err := getEnv("PING_INTERVAL_SECONDS", "60")
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

	ids, err := getEnv("SPEED_SERVER_IDS", "")
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

	secVal, err := getEnv("SPEED_INTERVAL_SECONDS", "3600")
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

func getEnv(key string, def string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok && def == "" {
		return "", fmt.Errorf("env var %s does not exist", key)
	}

	if value != "" {
		return value, nil
	}

	if def != "" {
		return def, nil
	}

	return "", fmt.Errorf("env var %s does not exist and no default value is set", key)
}
