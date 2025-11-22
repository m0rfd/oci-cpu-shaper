package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"oci-cpu-shaper/pkg/imds"
)

// IMDSConfig captures the metadata values exposed by the fake IMDS server.
type IMDSConfig struct {
	Region          string
	CanonicalRegion string
	InstanceID      string
	CompartmentID   string
	Shape           imds.ShapeConfig
}

// IMDSServer emulates the subset of IMDS endpoints exercised by the CLI.
type IMDSServer struct {
	server  *httptest.Server
	cfg     IMDSConfig
	handler http.HandlerFunc

	mu       sync.Mutex
	requests []string
}

// StartIMDSServer provisions a fake IMDS server and registers cleanup with the test harness.
func StartIMDSServer(tb testing.TB, cfg IMDSConfig) *IMDSServer {
	tb.Helper()

	handler := new(IMDSServer)
	handler.cfg = cfg

	server := httptest.NewServer(http.HandlerFunc(handler.serveHTTP))
	tb.Cleanup(server.Close)

	handler.server = server

	return handler
}

// StartIMDSHandlerServer provisions a fake IMDS server backed by a custom handler.
func StartIMDSHandlerServer(tb testing.TB, handler http.HandlerFunc) *IMDSServer {
	tb.Helper()

	serverHandler := new(IMDSServer)
	serverHandler.handler = handler

	server := httptest.NewServer(http.HandlerFunc(serverHandler.serveHTTP))
	tb.Cleanup(server.Close)

	serverHandler.server = server

	return serverHandler
}

// Endpoint returns the IMDS base URL suitable for OCI_CPU_SHAPER_IMDS_ENDPOINT.
func (s *IMDSServer) Endpoint() string {
	if s == nil || s.server == nil {
		return ""
	}

	return s.server.URL + path.Clean("/opc/v2")
}

// Requests returns a snapshot of observed IMDS paths.
func (s *IMDSServer) Requests() []string {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make([]string, len(s.requests))
	copy(snapshot, s.requests)

	return snapshot
}

func (s *IMDSServer) serveHTTP(writer http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, req.URL.Path)
	s.mu.Unlock()

	if s.handler != nil {
		s.handler(writer, req)

		return
	}

	switch strings.TrimPrefix(req.URL.Path, "/") {
	case "opc/v2/instance/region":
		s.writeText(writer, s.cfg.Region)
	case "opc/v2/instance", "opc/v2/instance/":
		payload := struct {
			RegionInfo struct {
				CanonicalRegionName string `json:"canonicalRegionName"`
			} `json:"regionInfo"`
		}{
			RegionInfo: struct {
				CanonicalRegionName string `json:"canonicalRegionName"`
			}{
				CanonicalRegionName: s.cfg.CanonicalRegion,
			},
		}

		s.writeJSON(writer, payload)
	case "opc/v2/instance/id":
		s.writeText(writer, s.cfg.InstanceID)
	case "opc/v2/instance/compartmentId":
		s.writeText(writer, s.cfg.CompartmentID)
	case "opc/v2/instance/shapeConfig":
		s.writeJSON(writer, s.cfg.Shape)
	default:
		http.NotFound(writer, req)
	}
}

func (s *IMDSServer) writeText(writer http.ResponseWriter, body string) {
	writer.Header().Set("Content-Type", "text/plain")
	_, _ = writer.Write([]byte(body))
}

func (s *IMDSServer) writeJSON(writer http.ResponseWriter, payload any) {
	writer.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(writer).Encode(payload)
	if err != nil {
		panic(err)
	}
}
