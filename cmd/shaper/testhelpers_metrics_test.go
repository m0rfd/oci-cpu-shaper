package main

import (
	"context"
	"errors"
)

const stubDefaultP95CPU = 0.25

var errMetricsServerBoom = errors.New("metrics server start failure")

type stubMetricsAdapter struct{}

func newStubMetricsClient() *stubMetricsAdapter { return &stubMetricsAdapter{} }

func (*stubMetricsAdapter) QueryP95CPU(context.Context, string) (float64, error) {
	return stubDefaultP95CPU, nil
}
