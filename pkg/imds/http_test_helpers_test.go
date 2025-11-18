package imds_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/imds"
)

const (
	regionResourcePath          = "/opc/v2/instance/region"
	instanceIDResourcePath      = "/opc/v2/instance/id"
	shapeConfigResourcePath     = "/opc/v2/instance/shapeConfig"
	canonicalRegionResourcePath = "/opc/v2/instance/"
	compartmentIDResourcePath   = "/opc/v2/instance/compartmentId"
	metadataAuthHeaderValue     = "Bearer Oracle"
	authorizationHeaderKey      = "Authorization"
)

func newIMDSTestClient(t *testing.T, responses map[string]string) *imds.HTTPClient {
	t.Helper()

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			requireIMDSAuthHeader(t, req)

			payload, ok := responses[req.URL.Path]
			if !ok {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			_, _ = writer.Write([]byte(payload))
		}),
	)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(httpClient, imds.WithBaseURL(server.URL+"/opc/v2"))

	httpMetadataClient, ok := client.(*imds.HTTPClient)
	if !ok {
		t.Fatalf("unexpected client type %T", client)
	}

	return httpMetadataClient
}

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

func requireNoError(t *testing.T, err error, msg string) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

func requireEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()

	if got != want {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
}

func requireIMDSAuthHeader(t *testing.T, req *http.Request) {
	t.Helper()

	got := req.Header.Get(authorizationHeaderKey)
	if got != metadataAuthHeaderValue {
		t.Fatalf("%s header = %q, want %q", authorizationHeaderKey, got, metadataAuthHeaderValue)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type cancelFuncKey struct{}

type faultyReadCloser struct {
	readErr  error
	closeErr error
}

func (f *faultyReadCloser) Read(_ []byte) (int, error) {
	return 0, f.readErr
}

func (f *faultyReadCloser) Close() error {
	return f.closeErr
}

type staticBody struct {
	data     []byte
	once     sync.Once
	closeErr error
}

func (s *staticBody) Read(p []byte) (int, error) {
	var bytesCopied int

	s.once.Do(func() {
		bytesCopied = copy(p, s.data)
	})

	if bytesCopied == 0 {
		return 0, io.EOF
	}

	return bytesCopied, io.EOF
}

func (s *staticBody) Close() error {
	return s.closeErr
}

func newHTTPClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       0,
	}
}

func newHTTPResponse(statusCode int, body io.ReadCloser, req *http.Request) *http.Response {
	return &http.Response{
		Status:           fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode:       statusCode,
		Proto:            "HTTP/1.1",
		ProtoMajor:       1,
		ProtoMinor:       1,
		Header:           make(http.Header),
		Body:             body,
		ContentLength:    -1,
		TransferEncoding: nil,
		Close:            false,
		Uncompressed:     false,
		Trailer:          nil,
		Request:          req,
		TLS:              nil,
	}
}
