// Package main is the entrypoint of the application.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof" // nolint:gosec

	_ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"

	"github.com/mantzas/netmon"
	"github.com/mantzas/netmon/otelsdk"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	httpPortName         = "NETMON_HTTP_PORT"
	httpPortDefaultValue = "8092"
)

const (
	serviceName = "netmon"
)

var serviceVersion = "0.1.0"

func main() {
	err := run()
	if err != nil {
		slog.Error("failed to run", "err", err)
	}
}

func run() error {
	port, err := getPort()
	if err != nil {
		return err
	}

	slog.Info("start monitoring", "port", port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otelShutdown, err := otelsdk.Setup(ctx, serviceName, serviceVersion)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	srv := createHTTPServer(port)

	srvErr := make(chan error, 1)

	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err = <-srvErr:
		return err
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		slog.Info("interrupts received, shutting down")
		stop()
	}

	ctx, cnl := context.WithTimeout(context.Background(), 10*time.Second)
	defer cnl()

	err = srv.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	slog.Info("server shutdown completed")
	return nil
}

func createHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	handleFunc := func(pattern string, hd func(http.ResponseWriter, *http.Request)) {
		otelHandler := otelhttp.NewHandler(http.HandlerFunc(hd), pattern)
		mux.Handle(pattern, otelHandler)
	}

	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	mux.HandleFunc("GET /health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("GET /ready", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handleFunc("GET /api/v1/ping/{ids}", pingHandlerFunc())
	handleFunc("GET /api/v1/speed/{ids}", speedHandlerFunc())

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           http.TimeoutHandler(mux, 59*time.Second, ""),
	}
}

type pingResponse struct {
	Results []netmon.PingResult `json:"results"`
}

func getServerIDs(r *http.Request) ([]string, error) {
	idsString := r.PathValue("ids")
	if idsString == "" {
		return nil, fmt.Errorf("ping failed. missing server ids value")
	}

	return strings.Split(idsString, ","), nil
}

func pingHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serverIDs, err := getServerIDs(r)
		if err != nil {
			slog.ErrorContext(r.Context(), "missing server ids in ping request", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		slog.InfoContext(r.Context(), "ping request", "server_ids", serverIDs)

		results, err := netmon.Ping(r.Context(), serverIDs)
		if err != nil {
			slog.ErrorContext(r.Context(), "ping failed", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response, err := json.Marshal(pingResponse{Results: results})
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to marshal results to JSON", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(response)
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to write response", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

type speedResponse struct {
	Results []netmon.SpeedResult `json:"results"`
}

func speedHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serverIDs, err := getServerIDs(r)
		if err != nil {
			slog.ErrorContext(r.Context(), "missing server ids in speed request", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		slog.InfoContext(r.Context(), "speed request", "server_ids", serverIDs)

		results := netmon.Speed(r.Context(), serverIDs)

		response, err := json.Marshal(speedResponse{Results: results})
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to marshal results to JSON", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(response)
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to write response", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func getPort() (int, error) {
	port, err := getEnv(httpPortName, httpPortDefaultValue)
	if err != nil {
		return 0, err
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("failed to convert port: %v", err)
	}

	return portInt, nil
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
