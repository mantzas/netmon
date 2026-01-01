// Package main is the entrypoint of the application.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/mantzas/netmon"
	"github.com/mantzas/netmon/otelsdk"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	serviceName = "netmon-cli"
	apiV1Prefix = "/api/v1/"
)

var (
	serviceVersion      = "0.1.0"
	serverIDsEnvName    = "NETMON_SPEED_SERVER_IDS"
	serverURLEnvVarName = "NETMON_SERVER_URL"
)

func main() {
	args, err := parseArguments()
	if err != nil {
		slog.Error("failed to parse flags", "err", err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx := context.Background()

	otelShutdown, err := otelsdk.Setup(ctx, serviceName, serviceVersion)
	if err != nil {
		slog.Error("failed to setup otel", "err", err)
		os.Exit(1)
	}

	err = executeRequest(ctx, args)
	err = errors.Join(err, otelShutdown(context.Background()))
	if err == nil {
		return
	}

	slog.Error("failed to execute request", "err", err)
	os.Exit(1)
}

type argument struct {
	cmd       string
	serverURL string
	serverIDs []string
}

func parseArguments() (argument, error) {
	var cmd string
	var serverIDsValue string
	var serverURL string
	flag.StringVar(&cmd, "cmd", "ping", "Can be either ping or speed.")
	flag.StringVar(&serverIDsValue, "servers", "5188", "A comma separated list of server IDs.")
	flag.StringVar(&serverURL, "url", "http://localhost:8092", "The URL of the netmon service.")
	flag.Parse()

	if cmd != "ping" && cmd != "speed" {
		return argument{}, fmt.Errorf("unknown cmd flag value: %s", cmd)
	}

	if url, ok := os.LookupEnv(serverURLEnvVarName); ok {
		serverURL = url
	}

	if ids, ok := os.LookupEnv(serverIDsEnvName); ok {
		serverIDsValue = ids
	}

	return argument{
		cmd:       cmd,
		serverIDs: strings.Split(serverIDsValue, ","),
		serverURL: serverURL,
	}, nil
}

func executeRequest(ctx context.Context, args argument) error {
	ctx, span := otel.Tracer(serviceName).Start(ctx, args.cmd)
	defer span.End()
	span.SetAttributes(attribute.String("cmd", args.cmd))
	span.SetAttributes(attribute.String("server_ids", strings.Join(args.serverIDs, ",")))

	targetURL := args.serverURL + apiV1Prefix + args.cmd + "/" + strings.Join(args.serverIDs, ",")

	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create %s request: %w", args.cmd, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "cmd", args.cmd, "err", err)
		}
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status code: %d for %s request", resp.StatusCode, args.cmd)
	}

	var resultsAttr slog.Attr

	switch args.cmd {
	case "ping":
		c := struct {
			Results []netmon.PingResult `json:"results"`
		}{}
		err = json.NewDecoder(resp.Body).Decode(&c)
		if err != nil {
			return fmt.Errorf("failed to decode ping response: %w", err)
		}

		resultsAttr = slog.Int("results", len(c.Results))

	case "speed":
		c := struct {
			Results []netmon.SpeedResult `json:"results"`
		}{}
		err = json.NewDecoder(resp.Body).Decode(&c)
		if err != nil {
			return fmt.Errorf("failed to decode ping response: %w", err)
		}
		resultsAttr = slog.Int("results", len(c.Results))
	}

	slog.InfoContext(ctx, "request executed successfully", slog.String("cmd", args.cmd), resultsAttr)
	return nil
}
