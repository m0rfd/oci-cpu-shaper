package status

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"oci-cpu-shaper/pkg/adapt"
)

type noopController struct{}

var errMarshalFail = errors.New("marshal fail")

func (noopController) Mode() string              { return "noop" }
func (noopController) State() adapt.State        { return adapt.StateNormal }
func (noopController) LastError() error          { return nil }
func (noopController) LastEstimatorError() error { return nil }

func TestServeHTTPNilController(t *testing.T) {
	t.Parallel()

	handler := NewHandler(nil, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", recorder.Code)
	}
}

func TestServeHTTPMarshalFallback(t *testing.T) {
	t.Parallel()

	handler := NewHandler(noopController{}, nil)
	handler.marshal = nil

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recorder.Code)
	}

	var snapshot Snapshot

	decodeErr := json.Unmarshal(recorder.Body.Bytes(), &snapshot)
	if decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}

	if snapshot.Mode != "noop" {
		t.Fatalf("expected noop mode, got %q", snapshot.Mode)
	}
}

func TestServeHTTPMarshalError(t *testing.T) {
	t.Parallel()

	handler := NewHandler(noopController{}, nil)
	handler.marshal = func(any) ([]byte, error) {
		return nil, errMarshalFail
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", recorder.Code)
	}
}
