//go:build !rootful

//nolint:testpackage // helper types for unexported worker hook tests.
package shape

type assertableError string

func (e assertableError) Error() string {
	return string(e)
}
