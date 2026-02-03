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
	"log/slog"
	"net/http"

	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	logger       *slog.Logger
	healthClient healthv1.HealthClient
}

// HealthHandlerBuilder is the builder for HealthHandler
type HealthHandlerBuilder struct {
	logger     *slog.Logger
	grpcClient *grpc.ClientConn
}

// NewHealthHandler creates a new HealthHandlerBuilder
func NewHealthHandler() *HealthHandlerBuilder {
	return &HealthHandlerBuilder{}
}

// SetLogger sets the logger for the health handler
func (b *HealthHandlerBuilder) SetLogger(logger *slog.Logger) *HealthHandlerBuilder {
	b.logger = logger
	return b
}

// SetGrpcClient sets the gRPC client connection for the health handler
func (b *HealthHandlerBuilder) SetGrpcClient(grpcClient *grpc.ClientConn) *HealthHandlerBuilder {
	b.grpcClient = grpcClient
	return b
}

// Build creates and returns the HealthHandler instance
func (b *HealthHandlerBuilder) Build() (*HealthHandler, error) {
	// Validate required fields
	if b.logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if b.grpcClient == nil {
		return nil, fmt.Errorf("gRPC client is required")
	}

	return &HealthHandler{
		logger:       b.logger,
		healthClient: healthv1.NewHealthClient(b.grpcClient),
	}, nil
}

// HandleHealth handles health check requests
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	response, err := h.healthClient.Check(r.Context(), &healthv1.HealthCheckRequest{})
	if err != nil {
		h.logger.ErrorContext(
			r.Context(),
			"Health check failed",
			slog.Any("error", err),
		)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if response.Status != healthv1.HealthCheckResponse_SERVING {
		h.logger.WarnContext(
			r.Context(),
			"Server is not serving",
			slog.String("status", response.Status.String()),
		)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
