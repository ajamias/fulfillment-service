/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"syscall"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"

	"github.com/innabox/fulfillment-common/network"
	"github.com/innabox/fulfillment-service/internal"
	"github.com/innabox/fulfillment-service/internal/handlers"
	shtdwn "github.com/innabox/fulfillment-service/internal/shutdown"
	"github.com/innabox/fulfillment-service/internal/version"
)

// NewConsoleProxyCommand creates and returns the `start console-proxy` command.
func NewConsoleProxyCommand() *cobra.Command {
	runner := &startConsoleProxyCommandRunner{}
	command := &cobra.Command{
		Use:   "console-proxy",
		Short: "Starts the console proxy",
		Args:  cobra.NoArgs,
		RunE:  runner.run,
	}
	flags := command.Flags()
	network.AddListenerFlags(flags, network.HttpListenerName, network.DefaultHttpAddress)
	network.AddListenerFlags(flags, network.MetricsListenerName, network.DefaultMetricsAddress)
	network.AddCorsFlags(flags, network.HttpListenerName)
	network.AddGrpcClientFlags(flags, network.GrpcClientName, network.DefaultGrpcAddress)
	flags.StringVar(
		&runner.args.externalAuthAddress,
		"grpc-authn-external-address",
		"",
		"Address of the external auth service using the Envoy ext_authz gRPC protocol.",
	)
	flags.StringSliceVar(
		&runner.args.caFiles,
		"ca-files",
		[]string{},
		"Files or directories containing trusted CA certificates in PEM format. "+
			"Used for TLS connections to the external auth service.",
	)
	flags.StringSliceVar(
		&runner.args.trustedTokenIssuers,
		"grpc-authn-trusted-token-issuers",
		[]string{},
		"Comma separated list of token issuers that are advertised as trusted by the gRPC server.",
	)
	return command
}

// startConsoleProxyCommandRunner contains the data and logic needed to run the `start console-proxy` command.
type startConsoleProxyCommandRunner struct {
	logger                        *slog.Logger
	flags                         *pflag.FlagSet
	grpcClient                    *grpc.ClientConn
	hostConsoleHandler            *handlers.HostConsoleHandler
	computeInstanceConsoleHandler *handlers.ComputeInstanceConsoleHandler
	healthHandler                 *handlers.HealthHandler
	args                          struct {
		caFiles             []string
		externalAuthAddress string
		trustedTokenIssuers []string
	}
}

// run runs the `start console-proxy` command.
func (c *startConsoleProxyCommandRunner) run(cmd *cobra.Command, argv []string) error {
	// Get the context:
	ctx, cancel := context.WithCancel(cmd.Context())

	// Get the dependencies from the context:
	c.logger = internal.LoggerFromContext(ctx)

	// Save the flags:
	c.flags = cmd.Flags()

	// Create the shutdown sequence:
	shutdown, err := shtdwn.NewSequence().
		SetLogger(c.logger).
		AddSignals(syscall.SIGTERM, syscall.SIGINT).
		AddContext("context", 0, cancel).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create shutdown sequence: %w", err)
	}

	// Create the network listener:
	c.logger.InfoContext(ctx, "Creating console proxy listener")
	listener, err := network.NewListener().
		SetLogger(c.logger).
		SetFlags(c.flags, network.HttpListenerName).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create console proxy listener", err)
	}

	// Load the trusted CA certificates:
	c.logger.InfoContext(ctx, "Loading trusted CA certificates")
	caPool, err := network.NewCertPool().
		SetLogger(c.logger).
		AddFiles(c.args.caFiles...).
		Build()
	if err != nil {
		return fmt.Errorf("failed to load trusted CA certificates: %w", err)
	}

	// Calculate the user agent:
	c.logger.InfoContext(ctx, "Calculating user agent")
	userAgent := fmt.Sprintf("%s/%s", consoleProxyUserAgent, version.Get())

	// Create the gRPC client:
	c.logger.InfoContext(ctx, "Creating gRPC client")
	c.grpcClient, err = network.NewGrpcClient().
		SetLogger(c.logger).
		SetFlags(c.flags, network.GrpcClientName).
		SetCaPool(caPool).
		SetUserAgent(userAgent).
		SetMetricsSubsystem("outbound").
		Build()
	if err != nil {
		return fmt.Errorf("failed to create gRPC client", err)
	}

	// Create the gateway multiplexer:
	c.logger.InfoContext(ctx, "Creating console proxy server")
	mux := http.NewServeMux()

	// Create the host console handler
	c.logger.InfoContext(ctx, "Creating Host console handler")
	c.hostConsoleHandler, err = handlers.NewHostConsoleHandler().
		SetLogger(c.logger).
		SetGrpcClient(c.grpcClient).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create Host console handler: %w", err)
	}

	// Create the compute instance console handler
	c.logger.InfoContext(ctx, "Creating ComputeInstance console handler")
	c.computeInstanceConsoleHandler, err = handlers.NewComputeInstanceConsoleHandler().
		SetLogger(c.logger).
		SetGrpcClient(c.grpcClient).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create ComputeInstance console handler: %w", err)
	}

	// Create the health handler
	c.logger.InfoContext(ctx, "Creating health handler")
	c.healthHandler, err = handlers.NewHealthHandler().
		SetLogger(c.logger).
		SetGrpcClient(c.grpcClient).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create health handler: %w", err)
	}

	// Add the console endpoints
	c.logger.InfoContext(ctx, "Adding console endpoints")
	mux.HandleFunc("POST /console/host/{id}", c.hostConsoleHandler.HandleConsole)
	mux.HandleFunc("POST /console/computeinstance/{id}", c.computeInstanceConsoleHandler.HandleConsole)

	// Add the health endpoint
	c.logger.InfoContext(ctx, "Adding health endpoint")
	mux.HandleFunc("/healthz", c.healthHandler.HandleHealth)

	// Add the CORS support:
	corsMiddleware, err := network.NewCorsMiddleware().
		SetLogger(c.logger).
		SetFlags(c.flags, network.HttpListenerName).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create CORS middleware: %w", err)
	}
	handler := corsMiddleware(mux)

	// Create the metrics server:
	c.logger.InfoContext(ctx, "Creating metrics listener")
	metricsListener, err := network.NewListener().
		SetLogger(c.logger).
		SetFlags(c.flags, network.MetricsListenerName).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create metrics listener: %w", err)
	}

	// Start the metrics server:
	c.logger.InfoContext(
		ctx,
		"Starting metrics server",
		slog.String("address", metricsListener.Addr().String()),
	)
	metricsServer := &http.Server{
		Addr:    metricsListener.Addr().String(),
		Handler: promhttp.Handler(),
	}
	go func() {
		err := metricsServer.Serve(metricsListener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.logger.ErrorContext(
				ctx,
				"Metrics server failed",
				slog.Any("error", err),
			)
		}
	}()
	shutdown.AddHttpServer(network.MetricsListenerName, 0, metricsServer)

	// Start serving:
	c.logger.InfoContext(
		ctx,
		"Start serving",
		slog.String("address", listener.Addr().String()),
	)
	http2Server := &http2.Server{}
	http1Server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: h2c.NewHandler(handler, http2Server),
	}
	go func() {
		err := http1Server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.logger.ErrorContext(
				ctx,
				"console proxy server failed",
				slog.Any("error", err),
			)
		}
	}()
	shutdown.AddHttpServer(network.HttpListenerName, 0, http1Server)

	// Keep running till the shutdown sequence finishes:
	c.logger.InfoContext(ctx, "Waiting for shutdown to sequence to complete")
	return shutdown.Wait()
}

// consoleProxyUserAgent is the user agent string for the console proxy
const consoleProxyUserAgent = "fulfillment-console-proxy"
