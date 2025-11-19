package status

import (
	"encoding/json"
	"net/http"
	"strings"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
)

// Controller exposes the status surface required by the health handler.
type Controller interface {
	Mode() string
	State() adapt.State
	LastError() error
	LastEstimatorError() error
}

// Snapshot captures the controller status returned by the handler.
type Snapshot struct {
	Mode           string          `json:"mode"`
	State          string          `json:"state"`
	LastOCIError   string          `json:"ociError"`
	EstimatorError string          `json:"estimatorError"`
	Cgroup         *CgroupSnapshot `json:"cgroup,omitempty"`
}

// CgroupSnapshot reports parsed cpu.weight and cpu.max values.
type CgroupSnapshot struct {
	Path      string             `json:"path"`
	CPUWeight *CgroupValue       `json:"cpuWeight,omitempty"`
	CPUMax    *CgroupMaxSnapshot `json:"cpuMax,omitempty"`
}

// CgroupValue holds a numeric setting or an associated error message.
type CgroupValue struct {
	Value *uint64 `json:"value,omitempty"`
	Error string  `json:"error,omitempty"`
}

// CgroupMaxSnapshot captures cpu.max quota/period tuples.
type CgroupMaxSnapshot struct {
	Quota     *uint64 `json:"quota,omitempty"`
	Period    *uint64 `json:"period,omitempty"`
	Unlimited *bool   `json:"unlimited,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// Handler renders controller health information as JSON.
type Handler struct {
	controller Controller
	cgroup     *cgroup.CPU
}

// NewHandler constructs a Handler that proxies controller status.
func NewHandler(controller Controller, cpuInfo *cgroup.CPU) *Handler {
	return &Handler{controller: controller, cgroup: cpuInfo}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	if h == nil || h.controller == nil {
		http.Error(writer, "controller unavailable", http.StatusServiceUnavailable)

		return
	}

	snapshot := Snapshot{
		Mode:           strings.TrimSpace(h.controller.Mode()),
		State:          h.controller.State().String(),
		LastOCIError:   "",
		EstimatorError: "",
		Cgroup:         buildCgroupSnapshot(h.cgroup),
	}

	lastOCIError := h.controller.LastError()
	if lastOCIError != nil {
		snapshot.LastOCIError = lastOCIError.Error()
	}

	estimatorErr := h.controller.LastEstimatorError()
	if estimatorErr != nil {
		snapshot.EstimatorError = estimatorErr.Error()
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(writer, "marshal status", http.StatusInternalServerError)

		return
	}

	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(payload)
}

func buildCgroupSnapshot(info *cgroup.CPU) *CgroupSnapshot {
	if info == nil {
		return nil
	}

	snapshot := new(CgroupSnapshot)
	snapshot.Path = strings.TrimSpace(info.Path)

	if info.Weight.Err != nil {
		value := new(CgroupValue)
		value.Error = info.Weight.Err.Error()
		snapshot.CPUWeight = value
	} else if info.Weight.Available {
		weight := info.Weight.Value
		value := new(CgroupValue)
		value.Value = &weight
		snapshot.CPUWeight = value
	}

	if info.Max.Err != nil {
		maxSnapshot := new(CgroupMaxSnapshot)
		maxSnapshot.Error = info.Max.Err.Error()
		snapshot.CPUMax = maxSnapshot
	} else if info.Max.Available {
		period := info.Max.Period
		unlimited := info.Max.Unlimited
		maxSnapshot := new(CgroupMaxSnapshot)
		maxSnapshot.Period = &period
		maxSnapshot.Unlimited = &unlimited

		if !info.Max.Unlimited {
			quota := info.Max.Quota
			maxSnapshot.Quota = &quota
		}

		snapshot.CPUMax = maxSnapshot
	}

	return snapshot
}
