package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteErrorHandlesScenarios(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns code", func(t *testing.T) {
		t.Parallel()

		const code = 7

		var buffer bytes.Buffer
		if got := writeError(&buffer, nil, code); got != code {
			t.Fatalf("unexpected code %d", got)
		}

		if buffer.Len() != 0 {
			t.Fatalf("expected no output, got %q", buffer.String())
		}
	})

	t.Run("writer succeeds", func(t *testing.T) {
		t.Parallel()

		const code = 11

		var buffer bytes.Buffer

		got := writeError(&buffer, errStubControllerRun, code)
		if got != code {
			t.Fatalf("unexpected code %d", got)
		}

		if !strings.Contains(buffer.String(), errStubControllerRun.Error()) {
			t.Fatalf("expected error message in buffer, got %q", buffer.String())
		}
	})

	t.Run("writer failure still returns code", func(t *testing.T) {
		t.Parallel()

		const code = 19

		writer := &failingWriter{writes: 0}

		got := writeError(writer, errStubControllerRun, code)
		if got != code {
			t.Fatalf("unexpected code %d", got)
		}

		if writer.writes != 1 {
			t.Fatalf("expected one write attempt, got %d", writer.writes)
		}
	})
}

type failingWriter struct {
	writes int
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	f.writes++

	return 0, errFailingWriter
}
