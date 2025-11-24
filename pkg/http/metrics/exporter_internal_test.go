package metrics

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var (
	errFailingBuffer  = errors.New("metrics: failing buffer")
	errBlankLastError = errors.New("   ")
)

const (
	lastErrorUnknown = "unknown"
	lastErrorNone    = "none"
)

type failingBuffer struct{}

func (f *failingBuffer) Write([]byte) (int, error) {
	return 0, errFailingBuffer
}

func (f *failingBuffer) Bytes() []byte {
	return nil
}

func TestExporterServeHTTPHandlesRenderErrors(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()
	exporter.bufferFactory = func() byteBuffer { return new(failingBuffer) }

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	exporter.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "write metrics") {
		t.Fatalf("expected render error to be reported, got %q", body)
	}
}

func TestExporterRenderRejectsNilBufferFactory(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()
	exporter.bufferFactory = func() byteBuffer { return nil }

	_, err := exporter.Render()
	if !errors.Is(err, errNilBuffer) {
		t.Fatalf("expected errNilBuffer, got %v", err)
	}
}

func TestExporterWriteToRejectsNilWriter(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()

	_, err := exporter.WriteTo(nil)
	if !errors.Is(err, errNilWriter) {
		t.Fatalf("expected errNilWriter, got %v", err)
	}
}

func TestExporterObserveHostCPUClampsOutOfRangeValues(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()

	exporter.ObserveHostCPU(-0.5)

	if snapshot := exporter.snapshot(); snapshot.hostCPUPercent != 0 {
		t.Fatalf(
			"expected negative utilisation to clamp to zero, got %.2f",
			snapshot.hostCPUPercent,
		)
	}

	exporter.ObserveHostCPU(math.NaN())

	if snapshot := exporter.snapshot(); snapshot.hostCPUPercent != 0 {
		t.Fatalf("expected NaN utilisation to reset to zero, got %.2f", snapshot.hostCPUPercent)
	}

	exporter.ObserveHostCPU(math.Inf(1))

	if snapshot := exporter.snapshot(); snapshot.hostCPUPercent != 0 {
		t.Fatalf("expected +Inf utilisation to reset to zero, got %.2f", snapshot.hostCPUPercent)
	}

	exporter.ObserveHostCPU(1.75)

	if snapshot := exporter.snapshot(); snapshot.hostCPUPercent != hundredPercent {
		t.Fatalf("expected utilisation to clamp to 100%%, got %.2f", snapshot.hostCPUPercent)
	}
}

func TestExporterSetIntervalClampsInvalidValues(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()

	exporter.SetInterval(-time.Second)

	if snapshot := exporter.snapshot(); snapshot.intervalSeconds != 0 {
		t.Fatalf("expected negative interval to clamp to zero, got %.2f", snapshot.intervalSeconds)
	}

	exporter.SetInterval(1500 * time.Millisecond)

	if snapshot := exporter.snapshot(); math.Abs(snapshot.intervalSeconds-1.5) > 1e-9 {
		t.Fatalf("expected interval seconds to capture 1.5, got %.6f", snapshot.intervalSeconds)
	}
}

func TestExporterSetLastErrorNormalizesMessages(t *testing.T) {
	t.Parallel()

	exporter := NewExporter()
	exporter.SetLastError(errBlankLastError)

	if snapshot := exporter.snapshot(); snapshot.lastError != lastErrorUnknown {
		t.Fatalf("expected blank error to normalize to unknown, got %q", snapshot.lastError)
	}

	exporter.SetLastError(nil)

	if snapshot := exporter.snapshot(); snapshot.lastError != lastErrorNone {
		t.Fatalf("expected nil error to normalize to none, got %q", snapshot.lastError)
	}
}
