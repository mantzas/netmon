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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	httpPortName                 = "NETMON_HTTP_PORT"
	httpPortDefaultValue         = "8092"
	otlpGRPCEndpointName         = "NETMON_OTLP_GRPC_ENDPOINT"
	otlpGRPCEndpointDefaultValue = "localhost:4317"
	speedServerIDs               = "NETMON_SPEED_SERVER_IDS"
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
	servers, err := getServerIDs()
	if err != nil {
		return err
	}

	port, err := getPort()
	if err != nil {
		return err
	}

	gRPCEndpoint, err := getGRPCEndpoint()
	if err != nil {
		return err
	}

	slog.Info("start monitoring", "port", port, "servers", servers)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otelShutdown, err := setupOTelSDK(ctx, serviceName, serviceVersion, gRPCEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	srv := createHTTPServer(port, servers)

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

func createHTTPServer(port int, servers []string) *http.Server {
	mux := http.NewServeMux()
	handleFunc := func(pattern string, hd func(http.ResponseWriter, *http.Request)) {
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(hd))
		otelHandler := otelhttp.NewHandler(handler, pattern)
		mux.Handle(pattern, otelHandler)
	}

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/ready", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handleFunc("/api/v1/ping", pingHandlerFunc(servers))
	handleFunc("/api/v1/speed", speedHandlerFunc(servers))

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

func pingHandlerFunc(serverIDs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

func speedHandlerFunc(serverIDs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

func getGRPCEndpoint() (string, error) {
	return getEnv(otlpGRPCEndpointName, otlpGRPCEndpointDefaultValue)
}

func getServerIDs() ([]string, error) {
	ids, err := getEnv(speedServerIDs, "")
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

func setupOTelSDK(ctx context.Context, serviceName, serviceVersion, gRPCEndpoint string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up resource.
	res, err := newResource(serviceName, serviceVersion)
	if err != nil {
		handleErr(err)
		return
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(ctx, gRPCEndpoint, res)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		))
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(ctx context.Context, endpoint string, res *resource.Resource) (*trace.TracerProvider, error) {
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithTimeout(5 * time.Second),
	}

	traceExporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(5*time.Second)),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)
	return traceProvider, nil
}
