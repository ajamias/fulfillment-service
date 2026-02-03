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
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

// CreateGrpcContextFromHttpRequest converts HTTP request headers to gRPC metadata context
// This is shared functionality used by all console handlers to forward authentication
// and other important headers to the gRPC server for proper authorization.
func CreateGrpcContextFromHttpRequest(r *http.Request) context.Context {
	md := metadata.MD{}

	// Extract and properly format Authorization header
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		md.Set("authorization", authHeader)
	}

	// Set required gRPC content-type
	md.Set("content-type", "application/grpc")

	// Forward essential headers that are safe for gRPC
	essentialHeaders := []string{"user-agent", "x-request-id", "x-forwarded-proto"}
	for _, headerName := range essentialHeaders {
		if values := r.Header[headerName]; len(values) > 0 {
			md.Set(strings.ToLower(headerName), values[0])
		}
	}

	return metadata.NewOutgoingContext(r.Context(), md)
}
