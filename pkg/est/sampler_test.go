//nolint:testpackage // tests exercise internal helpers for coverage
package est

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var errTestBoom = errors.New("test: boom")

type fakeSource struct {
	snapshots     []Snapshot
	err           error
	snapshotIndex int
}

type sequenceSource struct {
	responses []snapshotResponse
	index     int
}

type snapshotResponse struct {
	snapshot Snapshot
	err      error
}

func (f *fakeSource) Snapshot(_ context.Context) (Snapshot, error) {
	if f.err != nil {
		return Snapshot{}, f.err
	}

	if f.snapshotIndex >= len(f.snapshots) {
		if len(f.snapshots) == 0 {
			return Snapshot{Idle: 0, Total: 0, Runnable: 0}, nil
		}

		return f.snapshots[len(f.snapshots)-1], nil
	}

	snap := f.snapshots[f.snapshotIndex]
	f.snapshotIndex++

	return snap, nil
}

type SnapshotFunc func(context.Context) (Snapshot, error)

func (f SnapshotFunc) Snapshot(ctx context.Context) (Snapshot, error) {
	return f(ctx)
}

func (s *sequenceSource) Snapshot(_ context.Context) (Snapshot, error) {
	if len(s.responses) == 0 {
		return Snapshot{Idle: 0, Total: 0, Runnable: 0}, nil
	}

	if s.index >= len(s.responses) {
		return s.responses[len(s.responses)-1].snapshot, nil
	}

	response := s.responses[s.index]
	s.index++

	return response.snapshot, response.err
}

func TestSamplerEmitsObservations(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &fakeSource{snapshots: []Snapshot{
		{Idle: 10, Total: 20, Runnable: 0},
		{Idle: 12, Total: 30, Runnable: 0},
		{Idle: 13, Total: 40, Runnable: 0},
	}, err: nil, snapshotIndex: 0}

	sampler := NewSampler(source, time.Millisecond)
	sampler.now = func() time.Time { return time.Unix(0, 0) }

	observations := gatherObservations(t, sampler.Run(ctx), 2)

	cancel()

	const tolerance = 1e-9

	if diff := math.Abs(observations[0].Utilisation - 0.8); diff > tolerance {
		t.Fatalf("unexpected utilisation: got %.2f want %.2f", observations[0].Utilisation, 0.8)
	}

	if observations[0].BusyJiffies != 8 {
		t.Fatalf("unexpected busy jiffies: got %d want %d", observations[0].BusyJiffies, 8)
	}

	if observations[0].TotalJiffies != 10 {
		t.Fatalf("unexpected total jiffies: got %d want %d", observations[0].TotalJiffies, 10)
	}

	if observations[0].Runnable != 0 {
		t.Fatalf("expected runnable count to remain zero, got %.2f", observations[0].Runnable)
	}

	if diff := math.Abs(observations[1].Utilisation - 0.9); diff > tolerance {
		t.Fatalf("unexpected utilisation: got %.2f want %.2f", observations[1].Utilisation, 0.9)
	}
}

func gatherObservations(t *testing.T, observationsCh <-chan Observation, count int) []Observation {
	t.Helper()

	observations := make([]Observation, 0, count)
	timeout := time.After(100 * time.Millisecond)

	for len(observations) < count {
		select {
		case observation, ok := <-observationsCh:
			if !ok {
				t.Fatalf("channel closed prematurely; collected %d observations", len(observations))
			}

			if observation.Err != nil {
				t.Fatalf("unexpected error: %v", observation.Err)
			}

			observations = append(observations, observation)
		case <-timeout:
			t.Fatalf("timed out waiting for observations; collected %d", len(observations))
		}
	}

	return observations
}

func receiveObservation(
	t *testing.T,
	observations <-chan Observation,
	description string,
) Observation {
	t.Helper()

	select {
	case observation := <-observations:
		return observation
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", description)

		return Observation{
			Timestamp:    time.Time{},
			Utilisation:  0,
			Runnable:     0,
			BusyJiffies:  0,
			TotalJiffies: 0,
			Err:          nil,
		}
	}
}

func observationDeltaTestCases() []struct {
	name        string
	previous    Snapshot
	current     Snapshot
	utilisation float64
	runnable    float64
	busy        uint64
	total       uint64
} {
	return []struct {
		name        string
		previous    Snapshot
		current     Snapshot
		utilisation float64
		runnable    float64
		busy        uint64
		total       uint64
	}{
		{
			name:        "no-change",
			previous:    Snapshot{Idle: 10, Total: 20, Runnable: 0},
			current:     Snapshot{Idle: 10, Total: 20, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       0,
		},
		{
			name:        "full-busy",
			previous:    Snapshot{Idle: 10, Total: 20, Runnable: 0},
			current:     Snapshot{Idle: 10, Total: 40, Runnable: 0},
			utilisation: 1,
			runnable:    0,
			busy:        20,
			total:       20,
		},
		{
			name:        "wrap-around",
			previous:    Snapshot{Idle: 100, Total: 200, Runnable: 0},
			current:     Snapshot{Idle: 10, Total: 20, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       0,
		},
		{
			name:        "partial-busy",
			previous:    Snapshot{Idle: 40, Total: 100, Runnable: 0},
			current:     Snapshot{Idle: 50, Total: 140, Runnable: 0},
			utilisation: 0.75,
			runnable:    0,
			busy:        30,
			total:       40,
		},
	}
}

func TestBuildObservationHandlesDiverseDeltas(t *testing.T) {
	t.Parallel()

	for _, testCase := range observationDeltaTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			observation := buildObservation(time.Unix(0, 0), testCase.previous, testCase.current)

			assertObservation(
				t,
				observation,
				testCase.utilisation,
				testCase.runnable,
				testCase.busy,
				testCase.total,
			)
		})
	}
}

func TestBuildObservationNormalisesRunnables(t *testing.T) {
	t.Parallel()

	cpuCount := runtime.NumCPU()

	if cpuCount < 0 {
		t.Fatalf("invalid CPU count: %d", cpuCount)
	}

	runnable := uint64(cpuCount)

	observation := buildObservation(
		time.Unix(0, 0),
		Snapshot{Idle: 1, Total: 2, Runnable: 0},
		Snapshot{Idle: 1, Total: 12, Runnable: runnable * 2},
	)

	assertObservation(t, observation, 1, 2, 10, 10)
}

func TestBuildObservationClampsNonPositiveCPUCounts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cpuCount int
		previous Snapshot
		current  Snapshot
		util     float64
		runnable float64
		busy     uint64
		total    uint64
	}{
		{
			name:     "zero-cpu-count",
			cpuCount: 0,
			previous: Snapshot{Idle: 10, Total: 20, Runnable: 0},
			current:  Snapshot{Idle: 12, Total: 30, Runnable: 8},
			util:     0.8,
			runnable: 0,
			busy:     8,
			total:    10,
		},
		{
			name:     "negative-cpu-count",
			cpuCount: -1,
			previous: Snapshot{Idle: 40, Total: 120, Runnable: 0},
			current:  Snapshot{Idle: 50, Total: 150, Runnable: 12},
			util:     0.6666666666666666,
			runnable: 0,
			busy:     20,
			total:    30,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			observation := buildObservationWithCPUCount(
				time.Unix(0, 0),
				testCase.previous,
				testCase.current,
				testCase.cpuCount,
			)

			assertObservation(
				t,
				observation,
				testCase.util,
				testCase.runnable,
				testCase.busy,
				testCase.total,
			)
		})
	}
}

func TestBuildObservationHandlesZeroTotalDelta(t *testing.T) {
	t.Parallel()

	observation := buildObservation(
		time.Unix(0, 0),
		Snapshot{Idle: 40, Total: 120, Runnable: 0},
		Snapshot{Idle: 50, Total: 120, Runnable: 0},
	)

	assertObservation(t, observation, 0, 0, 0, 0)
}

func TestBuildObservationIdleDeltaExceedsTotal(t *testing.T) {
	t.Parallel()

	observation := buildObservation(
		time.Unix(0, 0),
		Snapshot{Idle: 10, Total: 20, Runnable: 0},
		Snapshot{Idle: 25, Total: 24, Runnable: 0},
	)

	assertObservation(t, observation, 0, 0, 0, 4)
}

func TestBuildObservationClampsDecreasingAndZeroDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		previous    Snapshot
		current     Snapshot
		utilisation float64
		runnable    float64
		busy        uint64
		total       uint64
	}{
		{
			name:        "decreasing-counters", // wrap-around clamped to zero delta
			previous:    Snapshot{Idle: 300, Total: 600, Runnable: 0},
			current:     Snapshot{Idle: 200, Total: 500, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       0,
		},
		{
			name:        "zero-delta", // unchanged snapshot keeps utilisation at zero
			previous:    Snapshot{Idle: 400, Total: 800, Runnable: 0},
			current:     Snapshot{Idle: 400, Total: 800, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			observation := buildObservation(time.Unix(0, 0), testCase.previous, testCase.current)

			assertObservation(
				t,
				observation,
				testCase.utilisation,
				testCase.runnable,
				testCase.busy,
				testCase.total,
			)

			if observation.Utilisation < 0 || observation.Utilisation > 1 {
				t.Fatalf("utilisation out of range: %.2f", observation.Utilisation)
			}
		})
	}
}

func assertObservation(
	t *testing.T,
	observation Observation,
	util, runnable float64,
	busy, total uint64,
) {
	t.Helper()

	if diff := math.Abs(observation.Utilisation - util); diff > 1e-9 {
		t.Fatalf("unexpected utilisation: got %.2f want %.2f", observation.Utilisation, util)
	}

	if diff := math.Abs(observation.Runnable - runnable); diff > 1e-9 {
		t.Fatalf("unexpected runnable: got %.2f want %.2f", observation.Runnable, runnable)
	}

	if observation.BusyJiffies != busy {
		t.Fatalf("unexpected busy: got %d want %d", observation.BusyJiffies, busy)
	}

	if observation.TotalJiffies != total {
		t.Fatalf("unexpected total: got %d want %d", observation.TotalJiffies, total)
	}
}

func TestBuildObservationClampsWrappedAndIdleDominantDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		previous    Snapshot
		current     Snapshot
		utilisation float64
		runnable    float64
		busy        uint64
		total       uint64
	}{
		{
			name:        "counter-wrap", // total decreased -> zero delta
			previous:    Snapshot{Idle: 100, Total: 200, Runnable: 0},
			current:     Snapshot{Idle: 90, Total: 150, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       0,
		},
		{
			name:        "idle-delta-exceeds-total", // idle delta larger than total delta -> clamp busy to zero
			previous:    Snapshot{Idle: 5, Total: 20, Runnable: 0},
			current:     Snapshot{Idle: 30, Total: 25, Runnable: 0},
			utilisation: 0,
			runnable:    0,
			busy:        0,
			total:       5,
		},
		{
			name:        "non-wrapping", // normal path remains unchanged
			previous:    Snapshot{Idle: 10, Total: 20, Runnable: 0},
			current:     Snapshot{Idle: 12, Total: 30, Runnable: 0},
			utilisation: 0.8,
			runnable:    0,
			busy:        8,
			total:       10,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			observation := buildObservation(time.Unix(0, 0), testCase.previous, testCase.current)

			assertObservation(
				t,
				observation,
				testCase.utilisation,
				testCase.runnable,
				testCase.busy,
				testCase.total,
			)

			if observation.Utilisation < 0 || observation.Utilisation > 1 {
				t.Fatalf("utilisation out of range: %.2f", observation.Utilisation)
			}
		})
	}
}

func TestParseCPUStat(t *testing.T) {
	t.Parallel()

	stat := "cpu  1 2 3 4 5 6 7 8 9 10\ncpu0 1 2 3 4 5 6 7 8 9 10\nprocs_running 5\n"

	snapshot, err := parseCPUStat(strings.NewReader(stat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshot.Total == 0 {
		t.Fatalf("expected total to be non-zero")
	}

	if snapshot.Idle != 9 {
		t.Fatalf("unexpected idle: got %d want 9", snapshot.Idle)
	}

	if snapshot.Runnable != 5 {
		t.Fatalf("unexpected runnable: got %d want 5", snapshot.Runnable)
	}
}

func TestFileSourceSnapshotContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	source := FileSource{Path: filepath.Join(t.TempDir(), "ignored")}

	_, err := source.Snapshot(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFileSourceSnapshotReadsProvidedPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")

	contents := "cpu  1 2 3 4 5 6 7 8 9 10\n"

	err := os.WriteFile(statPath, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write temp stat file: %v", err)
	}

	snap, err := (FileSource{Path: statPath}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	if snap.Total == 0 {
		t.Fatalf("expected total to be recorded")
	}

	if snap.Idle == 0 {
		t.Fatalf("expected idle jiffies to be recorded")
	}
}

func TestSamplerRunInitialSnapshotError(t *testing.T) {
	t.Parallel()

	sampler := NewSampler(
		&fakeSource{snapshots: nil, err: errTestBoom, snapshotIndex: 0},
		time.Millisecond,
	)
	sampler.now = func() time.Time { return time.Unix(123, 0) }

	ctx := context.Background()

	observations := sampler.Run(ctx)

	observation, ok := <-observations
	if !ok {
		t.Fatalf("expected error observation")
	}

	if observation.Err == nil || !strings.Contains(observation.Err.Error(), "initial snapshot") {
		t.Fatalf("expected initial snapshot error, got %v", observation.Err)
	}

	if observation.Timestamp != time.Unix(123, 0) {
		t.Fatalf("unexpected timestamp: %v", observation.Timestamp)
	}

	if _, ok := <-observations; ok {
		t.Fatalf("expected channel to be closed after error observation")
	}
}

func TestSamplerRunRejectsDoubleStart(t *testing.T) {
	t.Parallel()

	sampler := NewSampler(
		&fakeSource{
			snapshots:     []Snapshot{{Idle: 1, Total: 2, Runnable: 0}},
			err:           nil,
			snapshotIndex: 0,
		},
		time.Hour,
	)
	sampler.now = func() time.Time { return time.Unix(0, 0) }

	ctx, cancel := context.WithCancel(context.Background())
	first := sampler.Run(ctx)

	cancel()

	for {
		_, ok := <-first
		if !ok {
			break
		}
	}

	second := sampler.Run(context.Background())

	observation, ok := <-second
	if !ok {
		t.Fatalf("expected error observation from second run")
	}

	if !errors.Is(observation.Err, ErrSamplerAlreadyStarted) {
		t.Fatalf("expected ErrSamplerAlreadyStarted, got %v", observation.Err)
	}

	if _, ok := <-second; ok {
		t.Fatalf("expected second channel to be closed")
	}
}

func TestSamplerEmitsErrorObservationWhenLoopFails(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	source := SnapshotFunc(func(context.Context) (Snapshot, error) {
		count := calls.Add(1)
		if count == 1 {
			return Snapshot{Idle: 1, Total: 10, Runnable: 0}, nil
		}

		return Snapshot{}, errTestBoom
	})

	sampler := NewSampler(source, time.Millisecond)
	sampler.now = func() time.Time { return time.Unix(42, 0) }

	ctx := context.Background()

	observations := sampler.Run(ctx)

	select {
	case observation := <-observations:
		if observation.Err == nil {
			t.Fatalf("expected error observation, got %+v", observation)
		}

		if !strings.Contains(observation.Err.Error(), "sample snapshot") {
			t.Fatalf("expected sample snapshot error, got %v", observation.Err)
		}

		if observation.Timestamp != time.Unix(42, 0) {
			t.Fatalf("expected timestamp to use sampler clock, got %v", observation.Timestamp)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for error observation")
	}
}

func TestSamplerPublishObservationContextCancelled(t *testing.T) {
	t.Parallel()

	sampler := new(Sampler)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	observations := make(chan Observation)

	if sampler.publishObservation(ctx, observations, Observation{
		Timestamp:    time.Time{},
		Utilisation:  0.5,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	}) {
		t.Fatal("expected publishObservation to report cancellation")
	}

	select {
	case observation := <-observations:
		t.Fatalf("expected channel to remain empty, received %#v", observation)
	default:
	}
}

func TestSamplerTimeSourceFallbacksToNow(t *testing.T) {
	t.Parallel()

	var sampler Sampler

	nowFn := sampler.timeSource()
	if nowFn == nil {
		t.Fatal("expected timeSource to return a non-nil function")
	}

	before := time.Now()

	after := nowFn()
	if after.Before(before.Add(-time.Second)) || after.After(before.Add(5*time.Second)) {
		t.Fatalf("unexpected timestamp from fallback: %v", after)
	}
}

func TestParseCPUStatErrorCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		matches error
	}{
		{
			name:    "empty",
			input:   "",
			matches: io.EOF,
		},
		{
			name:    "unexpected prefix",
			input:   "cpu0 1 2 3\n",
			matches: ErrUnexpectedProcStatFormat,
		},
		{
			name:    "too few fields",
			input:   "cpu 1 2 3\n",
			matches: ErrProcStatTooShort,
		},
		{
			name:    "parse failure",
			input:   "cpu 1 two 3 4 5\n",
			matches: strconv.ErrSyntax,
		},
		{
			name:    "invalid procs_running",
			input:   "cpu 1 2 3 4 5\nprocs_running nope\n",
			matches: strconv.ErrSyntax,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseCPUStat(strings.NewReader(testCase.input))
			if err == nil {
				t.Fatalf("expected error for %s", testCase.name)
			}

			if !errors.Is(err, testCase.matches) {
				t.Fatalf("expected error to wrap %v, got %v", testCase.matches, err)
			}
		})
	}
}

func TestFileSourceSnapshotOpenFailure(t *testing.T) {
	t.Parallel()

	missingPath := filepath.Join(t.TempDir(), "missing.stat")
	source := FileSource{Path: missingPath}

	_, err := source.Snapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when opening missing file")
	}

	if !strings.Contains(err.Error(), "open") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestNewSamplerDefaultsInterval(t *testing.T) {
	t.Parallel()

	sampler := NewSampler(
		&fakeSource{
			snapshots:     []Snapshot{{Idle: 1, Total: 2, Runnable: 0}},
			err:           nil,
			snapshotIndex: 0,
		},
		0,
	)
	if sampler.interval != DefaultInterval {
		t.Fatalf("expected default interval %s, got %s", DefaultInterval, sampler.interval)
	}

	negative := NewSampler(
		&fakeSource{
			snapshots:     []Snapshot{{Idle: 1, Total: 2, Runnable: 0}},
			err:           nil,
			snapshotIndex: 0,
		},
		-time.Second,
	)
	if negative.interval != DefaultInterval {
		t.Fatalf(
			"expected negative interval to coerce to %s, got %s",
			DefaultInterval,
			negative.interval,
		)
	}
}

func TestFileSourceSnapshotParseError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stat")

	err := os.WriteFile(path, []byte("cpu0 bad data\n"), 0o600)
	if err != nil {
		t.Fatalf("write temp stat file: %v", err)
	}

	_, snapshotErr := (FileSource{Path: path}).Snapshot(context.Background())
	if snapshotErr == nil || !strings.Contains(snapshotErr.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", snapshotErr)
	}
}

func TestSampleLoopStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sampler := NewSampler(
		&fakeSource{
			snapshots:     []Snapshot{{Idle: 1, Total: 2, Runnable: 0}},
			err:           nil,
			snapshotIndex: 0,
		},
		time.Millisecond,
	)
	sampler.now = func() time.Time { return time.Unix(0, 0) }

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	observations := make(chan Observation, 1)

	sampler.sampleLoop(
		ctx,
		sampler.source,
		Snapshot{Idle: 1, Total: 2, Runnable: 0},
		ticker,
		observations,
	)

	select {
	case observation := <-observations:
		t.Fatalf("expected no observations after cancellation, got %+v", observation)
	default:
	}
}

func TestSampleLoopPublishesErrorAndContinues(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	observations, done, ticks := startSampleLoop(ctx, t)

	ticks <- time.Unix(0, 0)

	ticks <- time.Unix(0, 0)

	assertErrorObservation(t, observations)

	observation := receiveObservation(t, observations, "recovery observation")
	assertObservation(t, observation, 0.8, 0, 8, 10)

	cancel()

	assertLoopTermination(t, done)
}

func startSampleLoop(
	ctx context.Context,
	t *testing.T,
) (chan Observation, chan struct{}, chan time.Time) {
	t.Helper()

	ticks := make(chan time.Time)
	ticker := &time.Ticker{C: ticks}

	sampler := NewSampler(&sequenceSource{responses: []snapshotResponse{
		{snapshot: Snapshot{Idle: 0, Total: 0, Runnable: 0}, err: errTestBoom},
		{snapshot: Snapshot{Idle: 12, Total: 32, Runnable: 0}, err: nil},
	}, index: 0}, time.Millisecond)
	sampler.now = func() time.Time { return time.Unix(99, 0) }

	observations := make(chan Observation, 3)
	done := make(chan struct{})

	go func() {
		sampler.sampleLoop(
			ctx,
			sampler.source,
			Snapshot{Idle: 10, Total: 22, Runnable: 0},
			ticker,
			observations,
		)
		close(done)
	}()

	return observations, done, ticks
}

func assertErrorObservation(t *testing.T, observations <-chan Observation) {
	t.Helper()

	errorObservation := receiveObservation(t, observations, "error observation")

	if errorObservation.Err == nil ||
		!strings.Contains(errorObservation.Err.Error(), "sample snapshot") {
		t.Fatalf("expected sample snapshot error, got %v", errorObservation.Err)
	}
}

func assertLoopTermination(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sampler loop did not exit after cancellation")
	}
}

func TestBuildObservationZeroDelta(t *testing.T) {
	t.Parallel()

	previous := Snapshot{Idle: 10, Total: 50, Runnable: 0}
	current := Snapshot{Idle: 5, Total: 40, Runnable: 0} // simulate counter wrap

	observation := buildObservation(time.Unix(0, 0), previous, current)

	assertObservation(t, observation, 0, 0, 0, 0)
}

func TestDiffCounterHandlesWrap(t *testing.T) {
	t.Parallel()

	if diff := diffCounter(500, 100); diff != 0 {
		t.Fatalf("expected wrapped counter delta to be zero, got %d", diff)
	}

	observation := buildObservation(
		time.Unix(0, 0),
		Snapshot{Idle: 300, Total: 600, Runnable: 0},
		Snapshot{Idle: 200, Total: 100, Runnable: 0},
	)

	assertObservation(t, observation, 0, 0, 0, 0)
}

func TestBuildObservationNonPositiveCPUCount(t *testing.T) {
	t.Parallel()

	observation := buildObservationWithCPUCount(
		time.Unix(0, 0),
		Snapshot{Idle: 1, Total: 2, Runnable: 0},
		Snapshot{Idle: 1, Total: 12, Runnable: 50},
		0,
	)

	assertObservation(t, observation, 1, 0, 10, 10)
}
