package adapt

import "sync"

type recordingDutyCycler struct {
	mu     sync.Mutex
	target float64
}

//nolint:ireturn // callers require the DutyCycler interface seam for testing.
func newModeAwareDutyCycler(mode string, shaper DutyCycler) DutyCycler {
	if shaper == nil {
		return nil
	}

	if ModeEnforcesTargets(mode) {
		return shaper
	}

	recorder := &recordingDutyCycler{
		mu:     sync.Mutex{},
		target: shaper.Target(),
	}

	return recorder
}

func (r *recordingDutyCycler) SetTarget(target float64) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.target = target
	r.mu.Unlock()
}

func (r *recordingDutyCycler) Target() float64 {
	if r == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.target
}

func (r *recordingDutyCycler) ObserveHostLoad(float64, float64) {
	if r == nil {
		return
	}

	// dry-run wrapper intentionally ignores host load updates
}
