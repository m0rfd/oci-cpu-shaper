package shape

import "math"

func setPauseThresholds(pool *Pool, pause, resume float64) {
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

		pool.paused.Store(0)
	} else if resume > pause {
		resume = pause
	}

	pool.pauseThresholdBits.Store(math.Float64bits(pause))
	pool.resumeThresholdBits.Store(math.Float64bits(resume))
}

func setRunnableGuard(pool *Pool, threshold float64) {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		threshold = 0
	}

	pool.runnableGuardBits.Store(math.Float64bits(threshold))
}

func observeHostLoad(pool *Pool, utilisation, runnable float64) {
	var ok bool

	utilisation, ok = normaliseUtilisation(utilisation)
	if !ok {
		return
	}

	runnable = normaliseRunnable(runnable)

	if runnableGuardHit(pool, runnable) {
		pool.paused.Store(1)

		return
	}

	pause := math.Float64frombits(pool.pauseThresholdBits.Load())
	if pause <= 0 {
		pool.paused.Store(0)

		return
	}

	if utilisation >= pause {
		pool.paused.Store(1)

		return
	}

	resume := math.Float64frombits(pool.resumeThresholdBits.Load())
	if utilisation <= resume {
		pool.paused.Store(0)
	}
}

func isPaused(pool *Pool) bool {
	return pool.paused.Load() == 1
}

func runnableGuardHit(pool *Pool, runnable float64) bool {
	guard := math.Float64frombits(pool.runnableGuardBits.Load())

	return guard > 0 && runnable >= guard
}

func normaliseUtilisation(utilisation float64) (float64, bool) {
	if math.IsNaN(utilisation) || math.IsInf(utilisation, 0) {
		return 0, false
	}

	if utilisation < 0 {
		return 0, true
	}

	if utilisation > 1 {
		return 1, true
	}

	return utilisation, true
}

func normaliseRunnable(runnable float64) float64 {
	if math.IsNaN(runnable) || math.IsInf(runnable, 0) {
		return 0
	}

	if runnable < 0 {
		return 0
	}

	return runnable
}
