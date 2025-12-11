package shape

import (
	"context"
	"errors"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Pool drives a group of duty-cycle workers that consume CPU in short quanta.
type Pool struct {
	workers int
	quantum time.Duration

	busyFunc  func(time.Duration)
	sleepFunc func(time.Duration)
	yieldFunc func()

	tickerFactory func(time.Duration) ticker

	workerStartHook         func() error
	workerStartErrorHandler func(error)
	rootfulInitErr          error

	targetBits          atomic.Uint64
	pauseThresholdBits  atomic.Uint64
	resumeThresholdBits atomic.Uint64
	runnableGuardBits   atomic.Uint64
	paused              atomic.Uint32
}

//nolint:gochecknoglobals // test hook override for sched_idle initialization
var (
	trySchedIdleHookMu sync.RWMutex
	trySchedIdleHook   = trySchedIdle
)

//nolint:gochecknoglobals // overridable in tests to observe sched_idle warnings
var defaultWorkerStartErrorHandler = func(error) {}

func runTrySchedIdleHook() error {
	trySchedIdleHookMu.RLock()

	hook := trySchedIdleHook

	trySchedIdleHookMu.RUnlock()

	return hook()
}

// DefaultQuantum bounds the busy loop to a responsive interval.
const DefaultQuantum = time.Millisecond

const (
	minQuantum = time.Millisecond
	maxQuantum = 5 * time.Millisecond
)

var errInvalidWorkerCount = errors.New("shape: worker count must be positive")

// NewPool constructs a worker pool with the provided worker count and quantum duration.
func NewPool(workers int, quantum time.Duration) (*Pool, error) {
	if workers <= 0 {
		return nil, errInvalidWorkerCount
	}

	if quantum <= 0 {
		quantum = DefaultQuantum
	}

	if quantum < minQuantum {
		quantum = minQuantum
	}

	if quantum > maxQuantum {
		quantum = maxQuantum
	}

	poolInstance := new(Pool)
	poolInstance.workers = workers
	poolInstance.quantum = quantum
	poolInstance.busyFunc = busyWait
	poolInstance.sleepFunc = time.Sleep
	poolInstance.yieldFunc = runtime.Gosched
	poolInstance.tickerFactory = func(duration time.Duration) ticker {
		return &runtimeTicker{ticker: time.NewTicker(duration)}
	}
	poolInstance.SetWorkerStartErrorHandler(nil)
	poolInstance.SetTarget(0)
	poolInstance.SetPauseThresholds(0, 0)

	err := runTrySchedIdleHook()
	if err != nil {
		poolInstance.workerStartErrorHandler(err)
	}

	configureWorkerStartHook(poolInstance, err)

	if err != nil {
		poolInstance.rootfulInitErr = err
	}

	return poolInstance, nil
}

// Start launches the worker goroutines. The pool terminates when the context is cancelled.
func (p *Pool) Start(ctx context.Context) {
	for range p.workers {
		go p.worker(ctx)
	}
}

// Workers returns the number of worker goroutines managed by the pool.
func (p *Pool) Workers() int {
	return p.workers
}

// Quantum reports the duty-cycle quantum assigned to each worker.
func (p *Pool) Quantum() time.Duration {
	return p.quantum
}

// SetTarget updates the duty cycle target in the range [0,1].
func (p *Pool) SetTarget(target float64) {
	if math.IsNaN(target) {
		target = 0
	}

	if target < 0 {
		target = 0
	} else if target > 1 {
		target = 1
	}

	p.targetBits.Store(math.Float64bits(target))
}

// Target returns the current duty-cycle target.
func (p *Pool) Target() float64 {
	return math.Float64frombits(p.targetBits.Load())
}

// SetWorkerStartErrorHandler installs a hook invoked when the worker start hook fails.
//
// A nil handler resets the hook to a no-op.
func (p *Pool) SetWorkerStartErrorHandler(handler func(error)) {
	if handler == nil {
		handler = defaultWorkerStartErrorHandler
	}

	p.workerStartErrorHandler = handler
	if p.rootfulInitErr != nil {
		handler(p.rootfulInitErr)
		p.rootfulInitErr = nil
	}
}

// SetPauseThresholds configures the host utilisation band that pauses/resumes workers.
//
// Values outside [0,1] are clamped. A zero pause threshold disables the feature.
func (p *Pool) SetPauseThresholds(pause, resume float64) {
	setPauseThresholds(p, pause, resume)
}

// SetRunnableGuard configures the runnable-per-CPU threshold that immediately pauses workers.
//
// A zero threshold disables runnable-based pausing.
func (p *Pool) SetRunnableGuard(threshold float64) {
	setRunnableGuard(p, threshold)
}

// ObserveHostLoad toggles the paused state based on the configured thresholds.
func (p *Pool) ObserveHostLoad(utilisation, runnable float64) {
	observeHostLoad(p, utilisation, runnable)
}

// Paused returns true when the worker pool is temporarily suspended.
func (p *Pool) Paused() bool {
	return isPaused(p)
}
