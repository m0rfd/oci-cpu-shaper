//nolint:testpackage // test covers unexported worker hook stub.
package shape

import "testing"

func TestConfigureWorkerStartHookIsNoop(t *testing.T) {
	t.Parallel()

	configureWorkerStartHook(nil, nil)
}
