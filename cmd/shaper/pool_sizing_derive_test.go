package main

import "testing"

func TestDeriveWorkerCountFromOCPUs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ocpus    float64
		fallback int
		want     int
		applied  bool
		capped   bool
	}{
		{name: "fallback", ocpus: 0, fallback: 3, want: 3, applied: false, capped: false},
		{name: "negative", ocpus: -8, fallback: 4, want: 4, applied: false, capped: false},
		{name: "belowOne", ocpus: 0.25, fallback: 2, want: 1, applied: true, capped: false},
		{name: "fractionalCeil", ocpus: 2.75, fallback: 2, want: 3, applied: true, capped: false},
		{
			name:     "maxCap",
			ocpus:    64,
			fallback: 2,
			want:     maxAutoSizedWorkers,
			applied:  true,
			capped:   true,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertDeriveWorkerCountFromOCPUs(
				t,
				testCase.ocpus,
				testCase.fallback,
				testCase.want,
				testCase.applied,
				testCase.capped,
			)
		})
	}
}

func TestDeriveWorkerCountFromOCPUSBounds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ocpus    float64
		fallback int
		want     int
		applied  bool
		capped   bool
	}{
		{
			name:     "zeroOCPUsFallback",
			ocpus:    0,
			fallback: 5,
			want:     5,
			applied:  false,
			capped:   false,
		},
		{
			name:     "negativeOCPUsFallback",
			ocpus:    -3,
			fallback: 6,
			want:     6,
			applied:  false,
			capped:   false,
		},
		{
			name:     "fractionalBelowMinWorkers",
			ocpus:    0.4,
			fallback: 2,
			want:     minAutoSizedWorkers,
			applied:  true,
			capped:   false,
		},
		{
			name:     "aboveMaxWorkers",
			ocpus:    128,
			fallback: 1,
			want:     maxAutoSizedWorkers,
			applied:  true,
			capped:   true,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertDeriveWorkerCountFromOCPUs(
				t,
				testCase.ocpus,
				testCase.fallback,
				testCase.want,
				testCase.applied,
				testCase.capped,
			)
		})
	}
}

func assertDeriveWorkerCountFromOCPUs(
	t *testing.T,
	ocpus float64,
	fallback int,
	wantWorkers int,
	wantApplied bool,
	wantCapped bool,
) {
	t.Helper()

	got, applied, capped := deriveWorkerCountFromOCPUs(ocpus, fallback)
	if got != wantWorkers || applied != wantApplied || capped != wantCapped {
		t.Fatalf(
			"deriveWorkerCountFromOCPUs(%v, %d) = (%d,%t,%t), want (%d,%t,%t)",
			ocpus,
			fallback,
			got,
			applied,
			capped,
			wantWorkers,
			wantApplied,
			wantCapped,
		)
	}
}
