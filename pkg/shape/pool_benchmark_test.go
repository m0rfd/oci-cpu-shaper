//nolint:testpackage,ireturn // benchmarks need access to internal hooks and ticker interfaces
package shape

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type benchmarkCase struct {
	workers int
	quantum time.Duration
	target  float64
}

func BenchmarkPoolDutyCycle(b *testing.B) {
	cases := []benchmarkCase{
		{workers: 1, quantum: time.Millisecond, target: 0.25},
		{workers: 2, quantum: time.Millisecond, target: 0.5},
		{workers: 2, quantum: 5 * time.Millisecond, target: 0.65},
		{workers: 4, quantum: 5 * time.Millisecond, target: 0.8},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("workers=%d/quantum=%s/target=%.2f", tc.workers, tc.quantum, tc.target)
		b.Run(name, func(b *testing.B) {
			runPoolBenchmark(b, tc)
		})
	}
}

func benchmarkTicks(iterations int) int {
	if iterations <= 0 {
		return 1
	}

	return iterations
}

func newBenchmarkPool(b *testing.B, cfg benchmarkCase) *Pool {
	b.Helper()

	pool, err := NewPool(cfg.workers, cfg.quantum)
	if err != nil {
		b.Fatalf("unexpected error creating pool: %v", err)
	}

	return pool
}

type benchmarkRecorder struct {
	expectedBusy time.Duration

	busyTotal   atomic.Int64
	idleTotal   atomic.Int64
	busyCount   atomic.Int64
	driftTotals atomic.Int64
}

func newBenchmarkRecorder(cfg benchmarkCase) *benchmarkRecorder {
	expected := min(max(time.Duration(cfg.target*float64(cfg.quantum)), 0), cfg.quantum)

	return &benchmarkRecorder{
		expectedBusy: expected,
		busyTotal:    atomic.Int64{},
		idleTotal:    atomic.Int64{},
		busyCount:    atomic.Int64{},
		driftTotals:  atomic.Int64{},
	}
}

func (r *benchmarkRecorder) attach(pool *Pool) {
	pool.busyFunc = r.recordBusy
	pool.sleepFunc = r.recordIdle
	pool.yieldFunc = func() {}
}

func (r *benchmarkRecorder) recordBusy(duration time.Duration) {
	r.busyTotal.Add(int64(duration))
	r.busyCount.Add(1)

	drift := duration - r.expectedBusy
	if drift < 0 {
		drift = -drift
	}

	r.driftTotals.Add(int64(drift))
}

func (r *benchmarkRecorder) recordIdle(duration time.Duration) {
	r.idleTotal.Add(int64(duration))
}

func (r *benchmarkRecorder) report(
	b *testing.B,
	quantum time.Duration,
	target float64,
	tickStdDev float64,
) {
	b.Helper()

	totalBusy := time.Duration(r.busyTotal.Load())
	totalIdle := time.Duration(r.idleTotal.Load())

	totalTime := totalBusy + totalIdle
	if totalTime <= 0 {
		return
	}

	busySamples := r.busyCount.Load()

	avgDrift := float64(r.driftTotals.Load())
	if busySamples > 0 {
		avgDrift /= float64(busySamples)
	} else {
		avgDrift = 0
	}

	cpuUsagePct := float64(totalBusy) / float64(totalTime) * 100

	b.ReportMetric(cpuUsagePct, "cpu_pct")
	b.ReportMetric(avgDrift, "avg_drift_ns")
	b.ReportMetric(tickStdDev, "tick_stddev")
	b.ReportMetric(float64(quantum.Nanoseconds()), "quantum_ns")
	b.ReportMetric(target*100, "target_pct")
}

func runPoolBenchmark(b *testing.B, cfg benchmarkCase) {
	b.Helper()

	ticksPerWorker := benchmarkTicks(b.N)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := newBenchmarkPool(b, cfg)
	scheduler := newBenchmarkScheduler(ticksPerWorker, cfg.workers)
	recorder := newBenchmarkRecorder(cfg)

	recorder.attach(pool)
	pool.tickerFactory = func(duration time.Duration) ticker {
		return scheduler.newManualTicker(duration)
	}

	pool.Start(ctx)
	pool.SetTarget(cfg.target)

	<-scheduler.Ready()

	b.ResetTimer()
	scheduler.Start()
	scheduler.Wait()
	cancel()
	b.StopTimer()

	recorder.report(b, cfg.quantum, cfg.target, scheduler.TickStdDev())
}

type benchmarkScheduler struct {
	ticksPerTicker  int
	expectedTickers int

	startOnce sync.Once
	startCh   chan struct{}

	readyOnce sync.Once
	readyCh   chan struct{}

	wg sync.WaitGroup

	registerMu sync.Mutex
	nextIndex  int

	counts []atomic.Int64
}

func newBenchmarkScheduler(ticksPerTicker, expectedTickers int) *benchmarkScheduler {
	if ticksPerTicker <= 0 {
		ticksPerTicker = 1
	}

	if expectedTickers <= 0 {
		expectedTickers = 1
	}

	scheduler := &benchmarkScheduler{
		ticksPerTicker:  ticksPerTicker,
		expectedTickers: expectedTickers,
		startOnce:       sync.Once{},
		startCh:         make(chan struct{}),
		readyOnce:       sync.Once{},
		readyCh:         make(chan struct{}),
		wg:              sync.WaitGroup{},
		registerMu:      sync.Mutex{},
		nextIndex:       0,
		counts:          make([]atomic.Int64, expectedTickers),
	}

	return scheduler
}

func (s *benchmarkScheduler) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *benchmarkScheduler) Start() {
	s.startOnce.Do(func() {
		close(s.startCh)
	})
}

func (s *benchmarkScheduler) Wait() {
	s.wg.Wait()
}

func (s *benchmarkScheduler) TickStdDev() float64 {
	total := 0.0

	count := len(s.counts)
	if count == 0 {
		return 0
	}

	values := make([]float64, count)

	for i := range s.counts {
		value := float64(s.counts[i].Load())
		values[i] = value
		total += value
	}

	mean := total / float64(count)
	if mean == 0 {
		return 0
	}

	variance := 0.0

	for _, value := range values {
		diff := value - mean
		variance += diff * diff
	}

	variance /= float64(count)

	return math.Sqrt(variance) / mean
}

func (s *benchmarkScheduler) newManualTicker(time.Duration) *benchmarkTicker {
	index := s.registerTicker()
	manual := &benchmarkTicker{
		scheduler: s,
		index:     index,
		remaining: s.ticksPerTicker,
		ch:        make(chan time.Time),
		stopCh:    make(chan struct{}),
		stopOnce:  sync.Once{},
	}

	s.wg.Add(1)

	go manual.run()

	return manual
}

func (s *benchmarkScheduler) registerTicker() int {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()

	index := s.nextIndex
	s.nextIndex++

	if s.nextIndex == s.expectedTickers {
		s.readyOnce.Do(func() {
			close(s.readyCh)
		})
	}

	return index
}

func (s *benchmarkScheduler) record(index int, sent int64) {
	if index < len(s.counts) && index >= 0 {
		s.counts[index].Store(sent)
	}
}

type benchmarkTicker struct {
	scheduler *benchmarkScheduler
	index     int
	remaining int
	ch        chan time.Time
	stopCh    chan struct{}

	stopOnce sync.Once
}

func (t *benchmarkTicker) C() <-chan time.Time {
	return t.ch
}

func (t *benchmarkTicker) Stop() {
	t.stopOnce.Do(func() {
		close(t.stopCh)
	})
}

func (t *benchmarkTicker) run() {
	defer t.scheduler.wg.Done()

	<-t.scheduler.startCh

	var sent int64

	for sent < int64(t.remaining) {
		select {
		case <-t.stopCh:
			t.scheduler.record(t.index, sent)

			return
		case t.ch <- time.Time{}:
			sent++
		}
	}

	t.scheduler.record(t.index, sent)
}
