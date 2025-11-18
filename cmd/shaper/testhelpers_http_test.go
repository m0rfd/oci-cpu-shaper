package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}

	server := new(httptest.Server)
	server.Listener = listener
	server.Config = new(http.Server)
	server.Config.Handler = handler
	server.Config.ReadHeaderTimeout = 5 * time.Second
	server.Start()

	return server
}
