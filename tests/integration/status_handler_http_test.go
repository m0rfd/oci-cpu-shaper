//go:build integration

package integration

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

type statusStubController struct {
	mode   string
	state  adapt.State
	ociErr error
	estErr error
}

func (s *statusStubController) Mode() string              { return s.mode }
func (s *statusStubController) State() adapt.State        { return s.state }
func (s *statusStubController) LastError() error          { return s.ociErr }
func (s *statusStubController) LastEstimatorError() error { return s.estErr }

func TestStatusHandlerHTTPServer(t *testing.T) {
	t.Parallel()

	t.Run("controllerUnavailable", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(status.NewHandler(nil, nil))
		t.Cleanup(server.Close)

		response, err := server.Client().Get(server.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 Service Unavailable, got %d", response.StatusCode)
		}
	})

	t.Run("reportsCgroupSnapshots", func(t *testing.T) {
		t.Parallel()

		controller := &statusStubController{
			mode:   "daemon",
			state:  adapt.StateFallback,
			ociErr: errors.New("latest oci failure"),
			estErr: errors.New("stalled estimator"),
		}

		t.Run("weightAndMaxErrors", func(t *testing.T) {
			t.Parallel()

			cpuInfo := &cgroup.CPU{
				Path:   " /errors/ ", // trimmed by handler
				Weight: cgroup.Weight{Err: errors.New("missing cpu.weight")},
				Max:    cgroup.Max{Err: errors.New("missing cpu.max")},
			}

			snapshot := fetchStatusSnapshot(t, controller, cpuInfo)

			if snapshot.Mode != "daemon" {
				t.Fatalf("expected mode daemon, got %q", snapshot.Mode)
			}

			if snapshot.State != adapt.StateFallback.String() {
				t.Fatalf("expected state %s, got %s", adapt.StateFallback, snapshot.State)
			}

			if snapshot.LastOCIError != "latest oci failure" {
				t.Fatalf("expected OCI error to propagate, got %q", snapshot.LastOCIError)
			}

			if snapshot.EstimatorError != "stalled estimator" {
				t.Fatalf("expected estimator error to propagate, got %q", snapshot.EstimatorError)
			}

			if snapshot.Cgroup == nil {
				t.Fatalf("expected cgroup snapshot, got nil")
			}

			if snapshot.Cgroup.Path != "/errors/" {
				t.Fatalf("expected cgroup path to match input, got %q", snapshot.Cgroup.Path)
			}

			if snapshot.Cgroup.CPUWeight == nil || snapshot.Cgroup.CPUWeight.Error == "" {
				t.Fatalf("expected cpu.weight error, got %+v", snapshot.Cgroup.CPUWeight)
			}

			if snapshot.Cgroup.CPUMax == nil || snapshot.Cgroup.CPUMax.Error == "" {
				t.Fatalf("expected cpu.max error, got %+v", snapshot.Cgroup.CPUMax)
			}
		})

		t.Run("maxUnlimitedAndQuota", func(t *testing.T) {
			t.Parallel()

			cases := []struct {
				name        string
				cpuInfo     *cgroup.CPU
				expectQuota bool
			}{
				{
					name: "unlimited", // quota omitted when unlimited
					cpuInfo: &cgroup.CPU{
						Path: "/sys/fs/cgroup/unlimited",
						Weight: cgroup.Weight{
							Available: true,
							Value:     200,
						},
						Max: cgroup.Max{
							Available: true,
							Period:    100000,
							Unlimited: true,
						},
					},
					expectQuota: false,
				},
				{
					name: "quota",
					cpuInfo: &cgroup.CPU{
						Path: "/sys/fs/cgroup/quota",
						Weight: cgroup.Weight{
							Available: true,
							Value:     100,
						},
						Max: cgroup.Max{
							Available: true,
							Quota:     50000,
							Period:    100000,
							Unlimited: false,
						},
					},
					expectQuota: true,
				},
			}

			for _, tt := range cases {
				tt := tt

				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					snapshot := fetchStatusSnapshot(t, controller, tt.cpuInfo)

					if snapshot.Cgroup == nil || snapshot.Cgroup.CPUWeight == nil || snapshot.Cgroup.CPUMax == nil {
						t.Fatalf("expected full cgroup snapshot, got %+v", snapshot.Cgroup)
					}

					if snapshot.Cgroup.CPUWeight.Value == nil || *snapshot.Cgroup.CPUWeight.Value != tt.cpuInfo.Weight.Value {
						t.Fatalf("expected cpu.weight %d, got %+v", tt.cpuInfo.Weight.Value, snapshot.Cgroup.CPUWeight.Value)
					}

					if snapshot.Cgroup.CPUMax.Period == nil || *snapshot.Cgroup.CPUMax.Period != tt.cpuInfo.Max.Period {
						t.Fatalf("expected cpu.max period %d, got %+v", tt.cpuInfo.Max.Period, snapshot.Cgroup.CPUMax.Period)
					}

					if snapshot.Cgroup.CPUMax.Unlimited == nil || *snapshot.Cgroup.CPUMax.Unlimited != tt.cpuInfo.Max.Unlimited {
						t.Fatalf("expected cpu.max unlimited %t, got %+v", tt.cpuInfo.Max.Unlimited, snapshot.Cgroup.CPUMax.Unlimited)
					}

					if tt.expectQuota {
						if snapshot.Cgroup.CPUMax.Quota == nil || *snapshot.Cgroup.CPUMax.Quota != tt.cpuInfo.Max.Quota {
							t.Fatalf("expected cpu.max quota %d, got %+v", tt.cpuInfo.Max.Quota, snapshot.Cgroup.CPUMax.Quota)
						}
					} else if snapshot.Cgroup.CPUMax.Quota != nil {
						t.Fatalf("expected quota to be omitted when unlimited, got %+v", snapshot.Cgroup.CPUMax.Quota)
					}
				})
			}
		})
	})
}

func fetchStatusSnapshot(t *testing.T, controller status.Controller, cpuInfo *cgroup.CPU) status.Snapshot {
	t.Helper()

	server := httptest.NewServer(status.NewHandler(controller, cpuInfo))
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", response.StatusCode)
	}

	var snapshot status.Snapshot
	if decodeErr := json.NewDecoder(response.Body).Decode(&snapshot); decodeErr != nil {
		t.Fatalf("decode status payload: %v", decodeErr)
	}

	return snapshot
}
