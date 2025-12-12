package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	"oci-cpu-shaper/pkg/oci"
	"oci-cpu-shaper/pkg/oci/metricsclient"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	stubCompartmentID   = "ocid1.compartment.oc1..test"
	stubRegion          = "us-ashburn-1"
	stubCanonicalRegion = "us-phoenix-1"
	overrideRegion      = "us-chicago-1"
)

var (
	errStubControllerRun = errors.New("controller run failed")
	errRegionDown        = errors.New("region down")
	errInstanceDown      = errors.New("id down")
	errShapeDown         = errors.New("shape down")
)

type stubController struct {
	mode        string
	runErr      error
	runCalled   bool
	deadline    time.Time
	deadlineSet bool
	state       adapt.State
	lastErr     error
	estErr      error
}

func (c *stubController) Run(ctx context.Context) error {
	c.runCalled = true

	if deadline, ok := ctx.Deadline(); ok {
		c.deadline = deadline
		c.deadlineSet = true
	} else {
		c.deadline = time.Time{}
		c.deadlineSet = false
	}

	return c.runErr
}

func (c *stubController) Mode() string { return c.mode }

func (c *stubController) State() adapt.State {
	if c.state == 0 {
		return adapt.StateNormal
	}

	return c.state
}

func (c *stubController) LastError() error { return c.lastErr }

func (c *stubController) LastEstimatorError() error { return c.estErr }

type blockingController struct {
	mode    string
	state   adapt.State
	lastErr error
	estErr  error
}

func (c *blockingController) Run(ctx context.Context) error {
	<-ctx.Done()

	err := ctx.Err()
	if err == nil {
		err = context.Canceled
	}

	return fmt.Errorf("controller run: %w", err)
}

func (c *blockingController) Mode() string { return c.mode }

func (c *blockingController) State() adapt.State { return c.state }

func (c *blockingController) LastError() error { return c.lastErr }

func (c *blockingController) LastEstimatorError() error { return c.estErr }

type stubPoolStarter struct {
	startCount int
	workers    int
	quantum    time.Duration
}

func (s *stubPoolStarter) Start(context.Context) {
	s.startCount++
}

func (s *stubPoolStarter) Workers() int {
	if s.workers <= 0 {
		return 1
	}

	return s.workers
}

func (s *stubPoolStarter) Quantum() time.Duration {
	if s.quantum <= 0 {
		return time.Millisecond
	}

	return s.quantum
}

type recordingMetricsRecorder struct {
	modeCalls          int
	stateCalls         int
	targetCalls        int
	intervalCalls      int
	lastErrorCalls     int
	relaxedSuccesses   int
	ociCalls           int
	hostCalls          int
	mode               string
	state              string
	target             float64
	interval           time.Duration
	lastError          error
	ociValue           float64
	ociTimestamp       time.Time
	hostUtilisation    float64
	relaxedSuccessRuns []int
}

func (r *recordingMetricsRecorder) SetMode(mode string) {
	r.modeCalls++
	r.mode = mode
}

func (r *recordingMetricsRecorder) SetState(state string) {
	r.stateCalls++
	r.state = state
}

func (r *recordingMetricsRecorder) SetTarget(target float64) {
	r.targetCalls++
	r.target = target
}

func (r *recordingMetricsRecorder) ObserveOCIP95(value float64, fetchedAt time.Time) {
	r.ociCalls++
	r.ociValue = value
	r.ociTimestamp = fetchedAt
}

func (r *recordingMetricsRecorder) ObserveHostCPU(utilisation float64) {
	r.hostCalls++
	r.hostUtilisation = utilisation
}

func (r *recordingMetricsRecorder) SetInterval(interval time.Duration) {
	r.intervalCalls++
	r.interval = interval
}

func (r *recordingMetricsRecorder) SetLastError(err error) {
	r.lastErrorCalls++
	r.lastError = err
}

func (r *recordingMetricsRecorder) SetRelaxedSuccesses(count int) {
	r.relaxedSuccesses++
	r.relaxedSuccessRuns = append(r.relaxedSuccessRuns, count)
}

func (*stubPoolStarter) SetWorkerStartErrorHandler(func(error)) {}

type trackingPoolStarter struct {
	startCalls       int
	workerCalls      int
	quantumCalls     int
	workers          int
	quantum          time.Duration
	workerStartErr   error
	handlerSet       int
	errorHandlerFunc func(error)
}

func (s *trackingPoolStarter) Start(context.Context) {
	s.startCalls++

	if s.errorHandlerFunc != nil {
		s.errorHandlerFunc(s.workerStartErr)
	}
}

func (s *trackingPoolStarter) Workers() int {
	s.workerCalls++

	if s.workers <= 0 {
		return 1
	}

	return s.workers
}

func (s *trackingPoolStarter) Quantum() time.Duration {
	s.quantumCalls++

	if s.quantum <= 0 {
		return time.Millisecond
	}

	return s.quantum
}

func (s *trackingPoolStarter) SetWorkerStartErrorHandler(handler func(error)) {
	s.handlerSet++
	s.errorHandlerFunc = handler
}

type stubIMDSClient struct {
	region               string
	regionErr            error
	canonicalRegion      string
	canonicalRegionErr   error
	instanceID           string
	instanceErr          error
	compartmentID        string
	compartmentErr       error
	shape                imds.ShapeConfig
	shapeErr             error
	regionCalls          int
	canonicalRegionCalls int
	instanceCalls        int
	compartmentCalls     int
	shapeCalls           int
}

func (s *stubIMDSClient) Region(context.Context) (string, error) {
	s.regionCalls++

	return s.region, s.regionErr
}

func (s *stubIMDSClient) CanonicalRegion(context.Context) (string, error) {
	s.canonicalRegionCalls++

	return s.canonicalRegion, s.canonicalRegionErr
}

func (s *stubIMDSClient) InstanceID(context.Context) (string, error) {
	s.instanceCalls++

	return s.instanceID, s.instanceErr
}

func (s *stubIMDSClient) CompartmentID(context.Context) (string, error) {
	s.compartmentCalls++

	return s.compartmentID, s.compartmentErr
}

func (s *stubIMDSClient) ShapeConfig(context.Context) (imds.ShapeConfig, error) {
	s.shapeCalls++

	return s.shape, s.shapeErr
}

func newOfflineStubIMDS() *stubIMDSClient {
	return &stubIMDSClient{ //nolint:exhaustruct
		regionErr:          errRegionDown,
		canonicalRegionErr: errRegionDown,
		instanceErr:        errInstanceDown,
		compartmentErr:     errInstanceDown,
		shape: imds.ShapeConfig{
			OCPUs:                     0,
			MemoryInGBs:               0,
			BaselineOcpuUtilization:   "",
			BaselineOCPUs:             0,
			ThreadsPerCore:            0,
			NetworkingBandwidthInGbps: 0,
			MaxVnicAttachments:        0,
		},
		shapeErr: errShapeDown,
	}
}

func newLoggingStubIMDS(
	region string,
	regionErr error,
	canonicalRegion string,
	canonicalErr error,
	instanceID string,
	instanceErr error,
	compartmentID string,
	compartmentErr error,
	shape imds.ShapeConfig,
	shapeErr error,
) *stubIMDSClient {
	return &stubIMDSClient{ //nolint:exhaustruct
		region:             region,
		regionErr:          regionErr,
		canonicalRegion:    canonicalRegion,
		canonicalRegionErr: canonicalErr,
		instanceID:         instanceID,
		instanceErr:        instanceErr,
		compartmentID:      compartmentID,
		compartmentErr:     compartmentErr,
		shape:              shape,
		shapeErr:           shapeErr,
	}
}

func stubShapeConfig(ocpus, memory float64) imds.ShapeConfig {
	return imds.ShapeConfig{ //nolint:exhaustruct
		OCPUs:       ocpus,
		MemoryInGBs: memory,
	}
}

func contextWithStubMetrics(t *testing.T, metrics oci.MetricsClient) context.Context {
	t.Helper()

	return metricsclient.WithBuilder(
		context.Background(),
		func(string, string) (metricsclient.MetricsClient, error) {
			return metrics, nil
		},
	)
}

func contextWithAssertingMetricsFactory(
	t *testing.T,
	metrics oci.MetricsClient,
	wantCompartmentID string,
	wantRegion string,
) context.Context {
	t.Helper()

	return metricsclient.WithBuilder(
		context.Background(),
		func(compartmentID, region string) (metricsclient.MetricsClient, error) {
			if compartmentID != wantCompartmentID {
				t.Fatalf("unexpected compartment id: %s", compartmentID)
			}

			if region != wantRegion {
				t.Fatalf("unexpected region: %s", region)
			}

			return metrics, nil
		},
	)
}

func loadConfigStub() func(string) (runtimeconfig.Config, error) {
	return func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.CompartmentID = stubCompartmentID
		cfg.OCI.Region = "us-phoenix-1"
		cfg.OCI.Offline = true

		return cfg, nil
	}
}

func requireRunInvoked(t *testing.T, ctrl *stubController) {
	t.Helper()

	if ctrl == nil || !ctrl.runCalled {
		t.Fatalf("expected controller Run to be invoked")
	}
}

func requireDeadlineCaptured(t *testing.T, ctrl *stubController) {
	t.Helper()

	if ctrl == nil {
		t.Fatalf("controller stub is nil")

		return
	}

	if !ctrl.deadlineSet {
		t.Fatalf("expected controller Run to capture deadline")
	}

	if ctrl.deadline.IsZero() {
		t.Fatalf("expected controller Run deadline to be set")
	}
}
