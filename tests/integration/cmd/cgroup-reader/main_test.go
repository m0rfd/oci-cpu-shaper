package main

import (
	"strings"
	"testing"
)

func TestRootCgroupPath(t *testing.T) {
	t.Parallel()

	data := "0::/docker/123\n1:cpu:/docker/123\n"

	path, err := rootCgroupPath(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path != "/docker/123" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestRootCgroupPathMissing(t *testing.T) {
	t.Parallel()

	data := "1:cpu:/docker/123\n"

	_, err := rootCgroupPath(strings.NewReader(data))
	if err == nil {
		t.Fatalf("expected error when root entry missing")
	}
}
