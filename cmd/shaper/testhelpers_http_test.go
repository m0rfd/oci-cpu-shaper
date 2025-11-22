package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(handler)

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}

	server.Listener = listener
	server.Start()

	return server
}

// NewIPv4TestServer binds to an IPv4 loopback address for predictable scraping.
//
//nolint:thelper // exported helper delegates to internal helper that marks the caller as helper.
func NewIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	return newIPv4TestServer(t, handler)
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate tcp port: %v", err)
	}

	addr := listener.Addr().String()

	closeErr := listener.Close()
	if closeErr != nil {
		t.Fatalf("close listener: %v", closeErr)
	}

	return addr
}

// FreeTCPAddress returns an available loopback TCP address for test servers.
//
//nolint:thelper // exported helper delegates to internal helper that marks the caller as helper.
func FreeTCPAddress(t *testing.T) string {
	return freeTCPAddress(t)
}

func serveGETRequest(
	t *testing.T,
	handler http.Handler,
	target string,
) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)

	handler.ServeHTTP(recorder, req)

	return recorder
}
