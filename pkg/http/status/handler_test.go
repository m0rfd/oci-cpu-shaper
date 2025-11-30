package status_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	status "oci-cpu-shaper/pkg/http/status"
)

var (
	errMetricsUnavailable = errors.New("metrics unavailable")
	errEstimatorStalled   = errors.New("estimator stalled")
	errMissingWeight      = errors.New("missing weight")
	errMissingMax         = errors.New("missing max")
)

type stubController struct {
	mode   string
	state  adapt.State
	ociErr error
	estErr error
}

func (s *stubController) Mode() string              { return s.mode }
func (s *stubController) State() adapt.State        { return s.state }
func (s *stubController) LastError() error          { return s.ociErr }
func (s *stubController) LastEstimatorError() error { return s.estErr }

func TestHandlerReturnsSnapshot(t *testing.T) {
	t.Parallel()

	controller := &stubController{
		mode:   "dry-run",
		state:  adapt.StateFallback,
		ociErr: errMetricsUnavailable,
		estErr: errEstimatorStalled,
	}

	cpuInfo := &cgroup.CPU{
		Path: "/user.slice/shaper.scope",
		Weight: cgroup.Weight{
			Path:      "",
			Value:     160,
			Available: true,
			Err:       nil,
		},
		Max: cgroup.Max{
			Path:      "",
			Quota:     50000,
			Period:    100000,
			Unlimited: false,
			Available: true,
			Err:       nil,
		},
	}

	handler := status.NewHandler(controller, cpuInfo)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}

	var snapshot status.Snapshot

	decodeErr := json.Unmarshal(recorder.Body.Bytes(), &snapshot)
	if decodeErr != nil {
		t.Fatalf("failed to decode response: %v", decodeErr)
	}

	requireEqual(t, "mode", "dry-run", snapshot.Mode)
	requireEqual(t, "state", adapt.StateFallback.String(), snapshot.State)
	requireEqual(t, "ociError", errMetricsUnavailable.Error(), snapshot.LastOCIError)
	requireEqual(t, "estimatorError", errEstimatorStalled.Error(), snapshot.EstimatorError)
	requireCgroupPointers(t, snapshot.Cgroup)
	requireUint64Ptr(t, "cpuWeight", snapshot.Cgroup.CPUWeight.Value, 160)
	requireUint64Ptr(t, "cpuMaxQuota", snapshot.Cgroup.CPUMax.Quota, 50000)
	requireBoolPtr(t, "cpuMaxUnlimited", snapshot.Cgroup.CPUMax.Unlimited, false)
}

func TestHandlerWithoutControllerReturnsServiceUnavailable(t *testing.T) {
	t.Parallel()

	handler := status.NewHandler(nil, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", recorder.Code)
	}
}

func TestHandlerReportsCgroupErrors(t *testing.T) {
	t.Parallel()

	controller := &stubController{
		mode:   "dry-run",
		state:  adapt.StateNormal,
		ociErr: nil,
		estErr: nil,
	}
	cpuInfo := &cgroup.CPU{
		Path: "/test",
		Weight: cgroup.Weight{
			Path:      "",
			Value:     0,
			Available: false,
			Err:       errMissingWeight,
		},
		Max: cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    0,
			Unlimited: false,
			Available: false,
			Err:       errMissingMax,
		},
	}

	handler := status.NewHandler(controller, cpuInfo)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recorder.Code)
	}

	var snapshot status.Snapshot

	err := json.Unmarshal(recorder.Body.Bytes(), &snapshot)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	requireCgroupPointers(t, snapshot.Cgroup)

	if got := snapshot.Cgroup.CPUWeight.Error; got == "" {
		t.Fatalf("expected weight error, got %+v", snapshot.Cgroup.CPUWeight)
	}

	if got := snapshot.Cgroup.CPUMax.Error; got == "" {
		t.Fatalf("expected cpu.max error, got %+v", snapshot.Cgroup.CPUMax)
	}
}

func TestHandlerTrimsWhitespaceAndReportsErrors(t *testing.T) {
	t.Parallel()

	controller := &stubController{
		mode:   "  dry-run \t\n",
		state:  adapt.StateSuppressed,
		ociErr: errMetricsUnavailable,
		estErr: errEstimatorStalled,
	}

	handler := status.NewHandler(controller, nil)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recorder.Code)
	}

	var snapshot status.Snapshot

	err := json.Unmarshal(recorder.Body.Bytes(), &snapshot)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	requireEqual(t, "mode", "dry-run", snapshot.Mode)
	requireEqual(t, "state", adapt.StateSuppressed.String(), snapshot.State)
	requireEqual(t, "ociError", errMetricsUnavailable.Error(), snapshot.LastOCIError)
	requireEqual(t, "estimatorError", errEstimatorStalled.Error(), snapshot.EstimatorError)

	if snapshot.Cgroup != nil {
		t.Fatalf("expected cgroup snapshot to be omitted, got %+v", snapshot.Cgroup)
	}
}

func requireEqual[T comparable](t *testing.T, name string, want, got T) {
	t.Helper()

	if want != got {
		t.Fatalf("expected %s %v, got %v", name, want, got)
	}
}

func requireUint64Ptr(t *testing.T, name string, ptr *uint64, want uint64) {
	t.Helper()

	if ptr == nil || *ptr != want {
		t.Fatalf("expected %s %d, got %+v", name, want, ptr)
	}
}

func requireBoolPtr(t *testing.T, name string, ptr *bool, want bool) {
	t.Helper()

	if ptr == nil || *ptr != want {
		t.Fatalf("expected %s %t, got %+v", name, want, ptr)
	}
}

func requireCgroupPointers(t *testing.T, snapshot *status.CgroupSnapshot) {
	t.Helper()

	if snapshot == nil || snapshot.CPUWeight == nil || snapshot.CPUMax == nil {
		t.Fatalf("expected cgroup snapshot, got %+v", snapshot)
	}
}

func TestHandlerNilReceiver(t *testing.T) {
	t.Parallel()

	var handler *status.Handler

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", recorder.Code)
	}
}

func TestHandlerOmitsCgroupWhenUnavailable(t *testing.T) {
	t.Parallel()

	controller := &stubController{
		mode:   "dry-run",
		state:  adapt.StateNormal,
		ociErr: nil,
		estErr: nil,
	}
	handler := status.NewHandler(controller, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	var snapshot status.Snapshot

	err := json.Unmarshal(recorder.Body.Bytes(), &snapshot)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if snapshot.Cgroup != nil {
		t.Fatalf("expected cgroup snapshot to be omitted, got %+v", snapshot.Cgroup)
	}
}
