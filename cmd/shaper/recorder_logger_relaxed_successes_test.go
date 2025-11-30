package main

import (
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

func TestControllerRecorderLoggerSetRelaxedSuccessesDelegatesWithoutLogging(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	exporter := metricshttp.NewExporter()

	recorder := newRecorderLogger(logger, exporter)
	recorder.SetRelaxedSuccesses(1)
	recorder.SetRelaxedSuccesses(5)

	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("expected no log entries, got %+v", entries)
	}

	output, err := exporter.Render()
	if err != nil {
		t.Fatalf("render metrics: %v", err)
	}

	expectMetricsSnippets(t, string(output), []string{"controller_relaxed_successes 5"})
}

func TestControllerRecorderLoggerSetRelaxedSuccessesNoopWithoutDelegate(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	recorder := &controllerRecorderLogger{ //nolint:exhaustruct // nil delegate exercises noop path without altering state
		logger: logger,
	}

	var waitGroup sync.WaitGroup
	for index := range 4 {
		waitGroup.Add(1)

		go func(count int) {
			defer waitGroup.Done()

			recorder.SetRelaxedSuccesses(count)
		}(index)
	}

	waitGroup.Wait()

	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("expected no log entries with nil delegate, got %+v", entries)
	}
}
