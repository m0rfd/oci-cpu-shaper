package oci_test

import (
	"context"
	"math"
	"testing"

	"oci-cpu-shaper/pkg/oci"
)

func TestStaticMetricsClientQueryP95CPU(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		value      float64
		wantEquals bool
	}{
		{name: "zero", value: 0, wantEquals: true},
		{name: "positive", value: 0.42, wantEquals: true},
		{name: "negative", value: -0.13, wantEquals: true},
		{name: "nan", value: math.NaN(), wantEquals: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := oci.NewStaticMetricsClient(testCase.value)

			got, fetchedAt, err := client.QueryP95CPU(context.Background(), "ocid1.test")
			if err != nil {
				t.Fatalf("QueryP95CPU returned error: %v", err)
			}

			if fetchedAt.IsZero() {
				t.Fatalf("expected non-zero timestamp for static client")
			}

			if testCase.wantEquals {
				if got != testCase.value {
					t.Fatalf("expected %f, got %f", testCase.value, got)
				}

				return
			}

			if !math.IsNaN(got) {
				t.Fatalf("expected NaN, got %f", got)
			}
		})
	}
}
