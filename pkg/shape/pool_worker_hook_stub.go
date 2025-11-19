//go:build !rootful

package shape

func configureWorkerStartHook(*Pool, error) {}
