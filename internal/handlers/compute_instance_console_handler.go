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

package handlers

import (
	"fmt"
	"io"
	"log/slog"
	//"net"
	"net/http"
	"strings"
	//"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	fulfillmentv1 "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
)

// ComputeInstanceConsoleHandler handles console access requests for compute instances
type ComputeInstanceConsoleHandler struct {
	logger                 *slog.Logger
	computeInstancesClient fulfillmentv1.ComputeInstancesClient
}

// ComputeInstanceConsoleHandlerBuilder is the builder for ComputeInstanceConsoleHandler
type ComputeInstanceConsoleHandlerBuilder struct {
	logger     *slog.Logger
	grpcClient *grpc.ClientConn
}

// NewComputeInstanceConsoleHandler creates a new ComputeInstanceConsoleHandlerBuilder
func NewComputeInstanceConsoleHandler() *ComputeInstanceConsoleHandlerBuilder {
	return &ComputeInstanceConsoleHandlerBuilder{}
}

// SetLogger sets the logger for the compute instance console handler
func (b *ComputeInstanceConsoleHandlerBuilder) SetLogger(logger *slog.Logger) *ComputeInstanceConsoleHandlerBuilder {
	b.logger = logger
	return b
}

// SetGrpcClient sets the gRPC client connection for the compute instance console handler
func (b *ComputeInstanceConsoleHandlerBuilder) SetGrpcClient(grpcClient *grpc.ClientConn) *ComputeInstanceConsoleHandlerBuilder {
	b.grpcClient = grpcClient
	return b
}

// Build creates and returns the ComputeInstanceConsoleHandler instance
func (b *ComputeInstanceConsoleHandlerBuilder) Build() (*ComputeInstanceConsoleHandler, error) {
	// Validate required fields
	if b.logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if b.grpcClient == nil {
		return nil, fmt.Errorf("gRPC client is required")
	}

	return &ComputeInstanceConsoleHandler{
		logger:                 b.logger,
		computeInstancesClient: fulfillmentv1.NewComputeInstancesClient(b.grpcClient),
	}, nil
}

// HandleConsole handles console access requests for compute instances
func (h *ComputeInstanceConsoleHandler) HandleConsole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := r.PathValue("id")
	if id == "" {
		h.logger.ErrorContext(ctx, "Missing ComputeInstance id")
		http.Error(w, "Missing ComputeInstance id", http.StatusBadRequest)
		return
	}

	// Extract the Authorization header and token
	authHeaderValue := r.Header.Get("Authorization")
	if authHeaderValue == "" {
		http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
		return
	}
	if !strings.HasPrefix(authHeaderValue, "Bearer ") {
		http.Error(w, "Unauthorized: Invalid authorization scheme", http.StatusUnauthorized)
		return
	}

	// Create gRPC context with the extracted token
	md := metadata.New(map[string]string{
		"authorization": authHeaderValue,
		"content-type":  "application/grpc",
		"user-agent":    r.Header.Get("user-agent"),
	})
	grpcCtx := metadata.NewOutgoingContext(ctx, md)

	// Debug: Print what metadata we're sending to gRPC
	h.logger.InfoContext(ctx, "gRPC metadata created", "metadata", md)

	// Verify compute instance access
	response, err := h.computeInstancesClient.Get(grpcCtx, &fulfillmentv1.ComputeInstancesGetRequest{Id: id})
	if err != nil {
		h.logger.ErrorContext(
			ctx,
			"Could not retrieve ComputeInstance with id "+id,
			slog.Any("error", err),
		)
		http.Error(w, "Could not retrieve ComputeInstance with id "+id, http.StatusInternalServerError)
		return
	}
	if response.Object == nil {
		h.logger.ErrorContext(
			ctx,
			"ComputeInstance with id "+id+" does not exist",
			slog.Any("error", err),
		)
		http.Error(w, "ComputeInstance with id "+id+" does not exist", http.StatusNotFound)
		return
	}
	/*
	// TODO: verify the console is enabled for this compute instance
	if response.Object.Status.ConsoleState != ComputeInstanceConsoleState_COMPUTE_INSTANCE_CONSOLE_STATE_ON {
		h.logger.ErrorContext(
			ctx,
			"The console of ComputeInstance "+id+" is not enabled",
			slog.Any("error", err),
		)
		http.Error(w, "The console of ComputeInstance "+id+" is not enabled", http.StatusForbidden)
		return
	}
	*/

	// Upgrade HTTP connection to raw TCP
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		h.logger.ErrorContext(ctx, "HTTP connection does not support hijacking")
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to hijack HTTP connection", slog.Any("error", err))
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer func() {
		clientConn.Close()
		h.logger.InfoContext(ctx, "Client connection closed")
	}()

	// Send TCP upgrade response
	upgradeResponse := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: tcp\r\n" +
		"Connection: upgrade\r\n\r\n"
	if _, err := clientConn.Write([]byte(upgradeResponse)); err != nil {
		h.logger.ErrorContext(ctx, "Failed to send TCP upgrade response", slog.Any("error", err))
		return
	}

	// Echo mode for testing - echo back whatever the client sends
	h.logger.InfoContext(ctx, "Console echo mode established for testing",
		slog.String("compute_instance_id", id))

	// Simple echo implementation: read from client and write back to client
	buffer := make([]byte, 4096)
	for {
		n, err := clientConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				h.logger.InfoContext(ctx, "Client disconnected")
			} else {
				h.logger.DebugContext(ctx, "Connection closed",
					slog.Any("error", err))
			}
			break
		}

		// Echo the data back to the client
		_, writeErr := clientConn.Write(buffer[:n])
		if writeErr != nil {
			h.logger.DebugContext(ctx, "Failed to write echo data",
				slog.Any("error", writeErr))
			break
		}

		h.logger.DebugContext(ctx, "Echoed data",
			slog.Int("bytes", n))
	}

	// Original console forwarding code (commented out for testing)
	/*
	// Start bidirectional copying between client and console
	// For now, consoleEndpoint points to a headless Service to an EndpointSlice updated by the OSAC operator
	// I don't think this is scalable, so might have to use another proxy (sidecar?)
	consoleEndpoint := fmt.Sprintf("%s.fulfillment-console-endpoints:8000", id)
	consoleConn, err := net.Dial("tcp", consoleEndpoint)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to connect to console endpoint",
			slog.String("endpoint", consoleEndpoint),
			slog.Any("error", err))
		return
	}
	defer func() {
		consoleConn.Close()
		h.logger.InfoContext(ctx, "Console connection closed")
	}()

	h.logger.InfoContext(ctx, "Console tunnel established",
		slog.String("console_endpoint", consoleEndpoint))

	var wg sync.WaitGroup
	wg.Add(2)

	// Copy from client to console
	go func() {
		defer wg.Done()
		defer consoleConn.Close()

		bytes, err := io.Copy(consoleConn, clientConn)
		if err == nil {
			h.logger.InfoContext(ctx, "Client disconnected")
		} else if err != io.EOF {
			h.logger.DebugContext(ctx, "Connection closed",
				slog.Any("error", err),
				slog.Int64("bytes_copied", bytes))
		}
	}()

	// Copy from console to client
	go func() {
		defer wg.Done()
		defer clientConn.Close()

		bytes, err := io.Copy(clientConn, consoleConn)
		if err == nil {
			h.logger.InfoContext(ctx, "Console disconnected")
		} else if err != io.EOF {
			h.logger.DebugContext(ctx, "Connection closed",
				slog.Any("error", err),
				slog.Int64("bytes_copied", bytes))
		}
	}()

	wg.Wait()
	*/
}

