//nolint:testpackage // tests need direct access to the unexported stub hook.
package shape

import "testing"

func TestConfigureWorkerStartHookNoop(t *testing.T) {
	t.Parallel()

	configureWorkerStartHook(nil, nil)

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	configureWorkerStartHook(pool, assertableError("ignored"))
}

type assertableError string

func (a assertableError) Error() string {
	return string(a)
}
