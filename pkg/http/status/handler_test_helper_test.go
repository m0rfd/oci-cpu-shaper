package status //nolint:testpackage

// SetMarshalFunc overrides the JSON marshal function during tests.
func SetMarshalFunc(handler *Handler, fn func(any) ([]byte, error)) {
	handler.marshal = fn
}
