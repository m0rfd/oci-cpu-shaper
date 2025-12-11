//nolint:testpackage // helper exercises internal hook state
package shape

func withTrySchedIdleHook(hook func() error) func() {
	trySchedIdleHookMu.Lock()

	original := trySchedIdleHook
	trySchedIdleHook = hook

	trySchedIdleHookMu.Unlock()

	return func() {
		trySchedIdleHookMu.Lock()

		trySchedIdleHook = original

		trySchedIdleHookMu.Unlock()
	}
}
