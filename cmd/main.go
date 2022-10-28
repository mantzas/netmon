package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mantzas/netmon/log"
	"github.com/mantzas/netmon/ping"
	"github.com/mantzas/netmon/speed"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := &log.Log{}

	err := run(logger)
	if err != nil {
		logger.Fatalln(err)
	}
}

func run(logger log.Logger) error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}

	logger.Printf("starting monitoring: %v", cfg)

	ctx, cnl := context.WithCancel(context.Background())
	defer cnl()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, os.Interrupt, syscall.SIGTERM)

	pingMonitor, err := ping.New(logger, cfg.ping)
	if err != nil {
		return err
	}

	speedMonitor, err := speed.New(ctx, logger, cfg.speed)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}

	wg.Add(1)

	go func() {
		pingMonitor.Monitor(ctx)
		wg.Done()
	}()

	wg.Add(1)

	go func() {
		speedMonitor.Monitor(ctx)
		wg.Done()
	}()

	srv := createHTTPServer(cfg.httpPort)

	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			logger.Printf("failed to run HTTP listener: %v", err)
		}
	}()

	for {
		select {
		case sig := <-chSignal:
			logger.Printf("signal %v received\n", sig)
			err = srv.Close()
			if err != nil {
				return fmt.Errorf("failed to close HTTP server: %v", err)
			}
			cnl()
		case <-ctx.Done():
			wg.Wait()
			return nil
		}
	}
}

func createHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
}

type config struct {
	httpPort int
	ping     ping.Config
	speed    speed.Config
}

func configFromEnv() (config, error) {
	cfg := config{}

	httpPort, err := getEnv("HTTP_PORT")
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

func getPingConfig() (ping.Config, error) {
	var err error
	cfg := ping.Config{}

	url, err := getEnv("PING_ADDRESSES")
	if err != nil {
		return ping.Config{}, err
	}

	cfg.Addresses = strings.Split(url, ",")

	secVal, err := getEnv("PING_INTERVAL_SECONDS")
	if err != nil {
		return ping.Config{}, err
	}

	seconds, err := strconv.Atoi(secVal)
	if err != nil {
		return ping.Config{}, fmt.Errorf("failed to convert ping interval: %v", err)
	}

	cfg.Interval = time.Duration(seconds) * time.Second

	return cfg, nil
}

func getSpeedConfig() (speed.Config, error) {
	var err error
	cfg := speed.Config{}

	ids, err := getEnv("SPEED_SERVER_IDS")
	if err != nil {
		return speed.Config{}, err
	}

	for _, id := range strings.Split(ids, ",") {
		serverID, err := strconv.Atoi(id)
		if err != nil {
			return speed.Config{}, fmt.Errorf("failed to convert server id [%s]: %v", id, err)
		}

		cfg.ServerIDs = append(cfg.ServerIDs, serverID)
	}

	secVal, err := getEnv("SPEED_INTERVAL_SECONDS")
	if err != nil {
		return speed.Config{}, err
	}

	seconds, err := strconv.Atoi(secVal)
	if err != nil {
		return speed.Config{}, fmt.Errorf("failed to convert speed interval: %v", err)
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
