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

func observeHostLoad(pool *Pool, utilisation float64) {
	if math.IsNaN(utilisation) || math.IsInf(utilisation, 0) {
		return
	}

	if utilisation < 0 {
		utilisation = 0
	} else if utilisation > 1 {
		utilisation = 1
	}

	pause := math.Float64frombits(pool.pauseThresholdBits.Load())
	if pause <= 0 {
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
