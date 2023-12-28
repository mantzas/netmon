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
	servers, err := getServerIDs()
	if err != nil {
		return err
	}

	port, err := getPort()
	if err != nil {
		return err
	}

	slog.Info("start monitoring", "port", port, "servers", servers)

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, os.Interrupt, syscall.SIGTERM)

	srv := createHTTPServer(port, servers)

	go func() {
		err = srv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to run HTTP listener", "err", err)
			os.Exit(1)
		}
	}()

	sig := <-chSignal
	slog.Info("signal received", "sig", sig)

	ctx, cnl := context.WithTimeout(context.Background(), 10*time.Second)
	defer cnl()

	err = srv.Shutdown(ctx)
	if err != nil {
		slog.Info("failed to shutdown server", "err", err)
	}

	return err
}

func createHTTPServer(port int, servers []string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/ready", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/api/v1/ping", pingHandler)
	mux.HandleFunc("/api/v1/speed", speedHandlerFunc(servers))

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           http.TimeoutHandler(mux, 59*time.Second, ""),
	}
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	results, err := netmon.Ping(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "ping failed", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(results)
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

func speedHandlerFunc(serverIds []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results := netmon.Speed(r.Context(), serverIds)

		response, err := json.Marshal(results)
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
	port, err := getEnv("HTTP_PORT", "8092")
	if err != nil {
		return 0, err
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("failed to convert port: %v", err)
	}

	return portInt, nil
}

func getServerIDs() ([]string, error) {
	ids, err := getEnv("SPEED_SERVER_IDS", "")
	if err != nil {
		return nil, err
	}

	serverIDs := strings.Split(ids, ",")

	if len(serverIDs) == 0 {
		return nil, fmt.Errorf("no valid server ids provided in the env var")
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
