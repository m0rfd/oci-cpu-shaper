//go:build linux && rootful

package shape

func configureWorkerStartHook(pool *Pool, initErr error) {
	if pool == nil || initErr != nil {
		return
	}

	pool.workerStartHook = trySchedIdle
}
