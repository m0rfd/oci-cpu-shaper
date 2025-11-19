package shape

import (
	"context"
	"time"
)

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
