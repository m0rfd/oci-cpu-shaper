package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
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

func (*stubPoolStarter) SetWorkerStartErrorHandler(func(error)) {}

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
