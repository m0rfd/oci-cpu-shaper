//go:build !rootful

package shape

func trySchedIdle() error {
	return nil
}
