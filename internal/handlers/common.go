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
	"errors"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

func ExtractAuthToken(r *http.Request) (string, error) {
	authHeaderValue := r.Header.Get("Authorization")

	authHeaderValue = strings.TrimSpace(authHeaderValue)
	if authHeaderValue == "" {
		return "", errors.New("Unauthorized: Missing Authorization header")
	}

	splitToken = strings.Slice(authHeaderValue, "Bearer ")
	if len(splitToken) != 2 {
		return "", errors.New("Unauthorized: Invalid authorization scheme")
	}

	return splitToken[1], nil
}

func NewGrpcContextFromRequest(r *http.Request, token string) context.Context {
	md := metadata.New(map[string]string{
		"authorization": token,
		"content-type":  "application/grpc",
		"user-agent":    r.Header.Get("user-agent"),
	})
	return metadata.NewOutgoingContext(r.Context(), md)
}

func CreateTunnel(client net.Conn, console net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy from client to console
	go func() {
		defer wg.Done()
		defer console.Close()

		_, err := io.Copy(console, client)
		if err == nil {
			h.logger.InfoContext(ctx, "Client disconnected")
		} else if err != io.EOF {
			h.logger.DebugContext(ctx, "Connection closed", slog.Any("error", err))
		}
	}()

	// Copy from console to client
	go func() {
		defer wg.Done()
		defer client.Close()

		_, err := io.Copy(clientConn, consoleConn)
		if err == nil {
			h.logger.InfoContext(ctx, "Console disconnected")
		} else if err != io.EOF {
			h.logger.DebugContext(ctx, "Connection closed", slog.Any("error", err))
		}
	}()

	wg.Wait()
}

func TestEcho(client net.Conn) {

}
