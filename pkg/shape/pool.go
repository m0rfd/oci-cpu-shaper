package shape

import (
	"context"
	"errors"
	"math"
	"runtime"
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
	paused              atomic.Uint32
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

	err := trySchedIdle()
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
		handler = func(error) {}
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
	if math.IsNaN(pause) {
		pause = 0
	}

	if math.IsNaN(resume) {
		resume = 0
	}

	if pause < 0 {
		pause = 0
	} else if pause > 1 {
		pause = 1
	}

	if resume < 0 {
		resume = 0
	} else if resume > 1 {
		resume = 1
	}

	if pause == 0 {
		resume = 0

		p.paused.Store(0)
	} else if resume > pause {
		resume = pause
	}

	p.pauseThresholdBits.Store(math.Float64bits(pause))
	p.resumeThresholdBits.Store(math.Float64bits(resume))
}

// ObserveHostLoad toggles the paused state based on the configured thresholds.
func (p *Pool) ObserveHostLoad(utilisation float64) {
	if math.IsNaN(utilisation) || math.IsInf(utilisation, 0) {
		return
	}

	if utilisation < 0 {
		utilisation = 0
	} else if utilisation > 1 {
		utilisation = 1
	}

	pause := math.Float64frombits(p.pauseThresholdBits.Load())
	if pause <= 0 {
		return
	}

	if utilisation >= pause {
		p.paused.Store(1)

		return
	}

	resume := math.Float64frombits(p.resumeThresholdBits.Load())
	if utilisation <= resume {
		p.paused.Store(0)
	}
}

// Paused returns true when the worker pool is temporarily suspended.
func (p *Pool) Paused() bool {
	return p.paused.Load() == 1
}

func (p *Pool) worker(ctx context.Context) {
	quantum := p.quantum
	busyFn := p.busyFunc
	sleepFn := p.sleepFunc
	yieldFn := p.yieldFunc
	startHook := p.workerStartHook
	startErrorHandler := p.workerStartErrorHandler

	ticker := p.tickerFactory(quantum)
	defer ticker.Stop()

	if startHook != nil {
		err := startHook()
		if err != nil && startErrorHandler != nil {
			startErrorHandler(err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if p.Paused() {
				sleepFn(quantum)
				yieldFn()

				continue
			}

			target := p.Target()

			busyDuration := min(time.Duration(target*float64(quantum)), quantum)

			idleDuration := quantum - busyDuration

			if busyDuration > 0 {
				busyFn(busyDuration)
			} else {
				yieldFn()
			}

			if idleDuration > 0 {
				sleepFn(idleDuration)
			} else {
				yieldFn()
			}

			yieldFn()
		}
	}
}

func busyWait(duration time.Duration) {
	if duration <= 0 {
		return
	}

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		runtime.Gosched()
	}
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type runtimeTicker struct {
	ticker *time.Ticker
}

func (t *runtimeTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t *runtimeTicker) Stop() {
	t.ticker.Stop()
}
