//go:build integration

package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"go.uber.org/zap"
)

func TestStartMetricsWrappersIntegration(t *testing.T) {
	t.Parallel()

	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	serverAddr := freeTCPAddress(t)

	shutdownServer, err := startMetricsServer(serverCtx, zap.NewNop(), serverAddr, handler)
	if err != nil {
		t.Fatalf("startMetricsServer: %v", err)
	}

	if shutdownServer == nil {
		t.Fatal("expected shutdown function from startMetricsServer")
	}

	deps := defaultRunDeps()
	deps.startMetricsServer = startMetricsServer

	endpointCtx, endpointCancel := context.WithCancel(context.Background())
	defer endpointCancel()

	endpointShutdown, cancelEndpoint, err := startMetricsEndpoint(
		endpointCtx,
		deps,
		zap.NewNop(),
		freeTCPAddress(t),
		handler,
	)
	if err != nil {
		t.Fatalf("startMetricsEndpoint: %v", err)
	}

	if endpointShutdown == nil {
		t.Fatal("expected shutdown function from startMetricsEndpoint")
	}

	if cancelEndpoint == nil {
		t.Fatal("expected cancel function from startMetricsEndpoint")
	}

	cancelEndpoint()
	endpointShutdown(context.Background())

	serverCancel()
	shutdownServer(context.Background())

	var nilContext context.Context

	failingServerShutdown, serverErr := startMetricsServer(nilContext, zap.NewNop(), freeTCPAddress(t), handler)
	if !errors.Is(serverErr, errMetricsContextRequired) {
		t.Fatalf("expected wrapped context error, got %v", serverErr)
	}

	if failingServerShutdown != nil {
		t.Fatal("expected nil shutdown when startMetricsServer fails in integration")
	}

	failingEndpointShutdown, failingCancel, endpointErr := startMetricsEndpoint(
		nilContext,
		deps,
		zap.NewNop(),
		freeTCPAddress(t),
		handler,
	)
	if !errors.Is(endpointErr, errMetricsContextRequired) {
		t.Fatalf("expected wrapped context error, got %v", endpointErr)
	}

	if failingEndpointShutdown != nil || failingCancel != nil {
		t.Fatalf("expected nil shutdown and cancel when startMetricsEndpoint fails in integration, got %v and %v", failingEndpointShutdown, failingCancel)
	}
}
