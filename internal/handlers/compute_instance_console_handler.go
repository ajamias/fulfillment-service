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
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"google.golang.org/grpc"

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
	resourceID := r.PathValue("id")

	if resourceID == "" {
		h.logger.ErrorContext(r.Context(), "Missing compute instance ID in URL path")
		http.Error(w, "Missing compute instance ID", http.StatusBadRequest)
		return
	}

	// Create gRPC context with authorization headers
	ctx := CreateGrpcContextFromHttpRequest(r)

	// Verify compute instance access and console enablement
	err := h.verifyComputeInstanceAccess(ctx, resourceID)
	if err != nil {
		h.logger.ErrorContext(
			r.Context(),
			"Compute instance console access denied or not found",
			slog.String("compute_instance_id", resourceID),
			slog.Any("error", err),
		)
		http.Error(w, "Compute instance not found or console access denied", http.StatusNotFound)
		return
	}

	h.logger.InfoContext(
		r.Context(),
		"Compute instance console access granted",
		slog.String("compute_instance_id", resourceID),
	)

	// TODO: Implement console tunnel setup here
	// This is where you would:
	// 1. Parse connection upgrade headers
	// 2. Establish TCP connection to BMC
	// 3. Upgrade HTTP connection to raw TCP
	// 4. Start bidirectional copying between client and BMC

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"success","resource_type":"computeinstance","message":"Console access granted for compute instance %s"}`,
		resourceID)
}

// verifyComputeInstanceAccess fetches the compute instance to verify the user has access and console is enabled
func (h *ComputeInstanceConsoleHandler) verifyComputeInstanceAccess(ctx context.Context, computeInstanceID string) error {
	response, err := h.computeInstancesClient.Get(ctx, &fulfillmentv1.ComputeInstancesGetRequest{Id: computeInstanceID})
	if err != nil {
		return err
	}
	if response.Object == nil {
		return fmt.Errorf("compute instance object is nil")
	}
	// TODO: verify the console is enabled for this compute instance
	return nil
}
