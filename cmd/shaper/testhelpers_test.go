package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
)

func stubBuildInfo(version, commit, date string) buildinfo.Info {
	return buildinfo.Info{
		Version:   version,
		GitCommit: commit,
		BuildDate: date,
	}
}

func loadConfigStub() func(string) (runtimeConfig, error) {
	return func(string) (runtimeConfig, error) {
		cfg := defaultRuntimeConfig()
		cfg.OCI.CompartmentID = stubCompartmentID
		cfg.OCI.Region = "us-phoenix-1"
		cfg.OCI.Offline = true

		return cfg, nil
	}
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

func (*stubPoolStarter) SetWorkerStartErrorHandler(func(error)) {}

type stubMetricsAdapter struct{}

const stubDefaultP95CPU = 0.25

func newStubMetricsClient() *stubMetricsAdapter { return &stubMetricsAdapter{} }

func (*stubMetricsAdapter) QueryP95CPU(context.Context, string) (float64, error) {
	return stubDefaultP95CPU, nil
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(context.Background(), "tcp", testMetricsBind)
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

type stubP95Querier struct {
	value        float32
	err          error
	calls        int
	lastResource string
	lastLast7d   bool
}

func (s *stubP95Querier) QueryP95CPU(
	_ context.Context,
	resourceID string,
	last7d bool,
) (float32, error) {
	s.calls++
	s.lastResource = resourceID
	s.lastLast7d = last7d

	if s.err != nil {
		return 0, s.err
	}

	return s.value, nil
}

func newStubP95Querier(value float32, err error) *stubP95Querier {
	return &stubP95Querier{value: value, err: err} //nolint:exhaustruct
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

func fieldString(fields []zap.Field, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.String
		}
	}

	return ""
}

func fieldBool(fields []zap.Field, key string) (bool, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.BoolType {
			return field.Integer != 0, true
		}

		return false, true
	}

	return false, false
}

func fieldFloat(fields []zap.Field, key string) float64 {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.Float64Type {
			if field.Integer < 0 {
				return 0
			}

			return math.Float64frombits(uint64(field.Integer))
		}

		if field.Type == zapcore.Float32Type {
			if field.Integer < 0 || field.Integer > int64(maxUint32) {
				return 0
			}

			return float64(math.Float32frombits(uint32(field.Integer)))
		}
	}

	return 0
}

func fieldInt(fields []zap.Field, key string) (int64, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		switch field.Type { //nolint:exhaustive // integer types only
		case zapcore.Int8Type,
			zapcore.Int16Type,
			zapcore.Int32Type,
			zapcore.Int64Type:
			return field.Integer, true
		case zapcore.Uint8Type,
			zapcore.Uint16Type,
			zapcore.Uint32Type,
			zapcore.Uint64Type:
			return field.Integer, true
		default:
			return 0, false
		}
	}

	return 0, false
}

func fieldDuration(fields []zap.Field, key string) (time.Duration, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.DurationType {
			return time.Duration(field.Integer), true
		}

		return 0, true
	}

	return 0, false
}

func requireLogFieldString(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()

	if got := fieldString(entry.Context, key); got != want {
		t.Fatalf("expected %s field %q, got %+v", key, want, entry.Context)
	}
}

func requireLogFieldFloat(t *testing.T, entry observer.LoggedEntry, key string, want float64) {
	t.Helper()

	if got := fieldFloat(entry.Context, key); got != want {
		t.Fatalf("expected %s field %v, got %+v", key, want, entry.Context)
	}
}

func requireSingleEntry(
	t *testing.T,
	observed *observer.ObservedLogs,
	level zapcore.Level,
) observer.LoggedEntry {
	t.Helper()

	entries := observed.FilterLevelExact(level).All()
	if len(entries) == 0 {
		t.Fatalf("expected %s log entry, got %+v", level, observed.All())
	}

	return entries[0]
}

var (
	errStubLoggerBoom    = errors.New("logger failure")
	errStubControllerRun = errors.New("controller run failed")
	errRegionDown        = errors.New("region down")
	errInstanceDown      = errors.New("id down")
	errShapeDown         = errors.New("shape down")
	errStubPrincipal     = errors.New("stub: principal client")
	errStubQueryFailure  = errors.New("stub: query failure")
	errFailingWriter     = errors.New("failing writer: write failed")
	errMetricsServerBoom = errors.New("metrics server start failure")
	errCgroupWeightBoom  = errors.New("read cpu.weight: boom")
	errCgroupMaxBoom     = errors.New("read cpu.max: boom")
)

const (
	maxUint32         = ^uint32(0)
	stubCompartmentID = "ocid1.compartment.oc1..test"
	stubRegion        = "us-ashburn-1"
	overrideRegion    = "us-chicago-1"
	imdsAuthHeaderKey = "Authorization"
	imdsAuthHeaderVal = "Bearer Oracle"
	metricsServerWait = time.Second
	testMetricsBind   = "127.0.0.1:0"
)
