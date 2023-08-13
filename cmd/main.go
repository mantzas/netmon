// Package main is the entrypoint of the application.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mantzas/netmon"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	err := run()
	if err != nil {
		slog.Error("failed to run", "err", err)
	}
}

func run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}

	slog.Info("start monitoring", "cfg", cfg)

	ctx, cnl := context.WithCancel(context.Background())
	defer cnl()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, os.Interrupt, syscall.SIGTERM)

	// Get the first measurements.
	err = netmon.SpeedTest(ctx, cfg.serverIDs, false)
	if err != nil {
		return err
	}

	srv := createHTTPServer(cfg.httpPort)

	wg := sync.WaitGroup{}

	wg.Add(1)

	go func() {
		defer wg.Done()
		err = srv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to run HTTP listener", "err", err)
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()
		process(ctx, cfg.pingInterval, cfg.speedInterval, cfg.serverIDs)
	}()

	sig := <-chSignal
	slog.Info("signal received", "sig", sig)

	err = srv.Close()
	if err != nil {
		slog.Info("failed to close HTTP listener", "err", err)
	}

	cnl()
	wg.Wait()

	return err
}

func process(ctx context.Context, pingInterval, speedInterval time.Duration, serverIDs []int) {
	pingTicker := time.NewTicker(pingInterval)
	speedTicker := time.NewTicker(speedInterval)

	for {
		select {
		case <-ctx.Done():
			pingTicker.Stop()
			speedTicker.Stop()
			return
		case <-pingTicker.C:
			pingTicker.Stop()
			err := netmon.SpeedTest(ctx, serverIDs, true)
			if err != nil {
				slog.ErrorContext(ctx, "speed test (ping only) failed", "err", err)
			}
			pingTicker.Reset(pingInterval)

		case <-speedTicker.C:
			speedTicker.Stop()
			err := netmon.SpeedTest(ctx, serverIDs, false)
			if err != nil {
				slog.ErrorContext(ctx, "speed test failed", "err", err)
			}
			speedTicker.Reset(speedInterval)
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
	httpPort      int
	serverIDs     []int
	pingInterval  time.Duration
	speedInterval time.Duration
}

func (c config) String() string {
	return fmt.Sprintf("port: %d server ids: %v ping interval: %v speed interval: %v", c.httpPort, c.serverIDs, c.pingInterval, c.speedInterval)
}

func configFromEnv() (config, error) {
	cfg := config{}

	httpPort, err := getEnv("HTTP_PORT", "8092")
	if err != nil {
		return cfg, err
	}

	cfg.httpPort, err = strconv.Atoi(httpPort)
	if err != nil {
		return cfg, err
	}

	cfg.pingInterval, err = getInterval("PING_INTERVAL_SECONDS", "60")
	if err != nil {
		return config{}, err
	}

	cfg.speedInterval, err = getInterval("SPEED_INTERVAL_SECONDS", "3600")
	if err != nil {
		return config{}, err
	}

	cfg.serverIDs, err = getServerIDs()
	if err != nil {
		return config{}, err
	}

	return cfg, nil
}

func getInterval(envVar, defaultValue string) (time.Duration, error) {
	var err error

	secVal, err := getEnv(envVar, defaultValue)
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.Atoi(secVal)
	if err != nil {
		return 0, fmt.Errorf("failed to convert ping interval: %v", err)
	}

	return time.Duration(seconds) * time.Second, nil
}

func getServerIDs() ([]int, error) {
	serverIDs := make([]int, 0)

	ids, err := getEnv("SPEED_SERVER_IDS", "")
	if err != nil {
		return nil, err
	}

	for _, id := range strings.Split(ids, ",") {
		serverID, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("failed to convert server id [%s]: %v", id, err)
		}

		serverIDs = append(serverIDs, serverID)
	}

	return serverIDs, nil
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
