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
