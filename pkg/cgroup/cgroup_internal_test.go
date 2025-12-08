package cgroup

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWeightHandlesEmptyAndInvalid(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	var err error

	emptyPath := filepath.Join(tmpDir, "empty")

	err = os.WriteFile(emptyPath, []byte("\n"), 0o600)
	if err != nil {
		t.Fatalf("write empty weight file: %v", err)
	}

	weight := readWeight(emptyPath)
	if weight.Err == nil || !strings.Contains(weight.Err.Error(), "empty value") {
		t.Fatalf("expected empty value error, got %+v", weight)
	}

	badPath := filepath.Join(tmpDir, "invalid")

	err = os.WriteFile(badPath, []byte("abc\n"), 0o600)
	if err != nil {
		t.Fatalf("write invalid weight file: %v", err)
	}

	weight = readWeight(badPath)
	if weight.Err == nil || !strings.Contains(weight.Err.Error(), "invalid syntax") {
		t.Fatalf("expected parse error, got %+v", weight)
	}
}

func TestReadMaxHandlesInvalidFormats(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	cases := []struct {
		name     string
		contents string
		expect   string
	}{
		{name: "empty", contents: "\n", expect: "empty value"},
		{name: "missing fields", contents: "50000\n", expect: "expected two fields"},
		{name: "invalid quota", contents: "oops 100000\n", expect: "invalid syntax"},
		{name: "invalid period", contents: "max not-a-number\n", expect: "invalid syntax"},
	}

	for _, rawCase := range cases {
		testCase := rawCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(tmpDir, strings.ReplaceAll(testCase.name, " ", "_"))

			err := os.WriteFile(path, []byte(testCase.contents), 0o600)
			if err != nil {
				t.Fatalf("write cpu.max: %v", err)
			}

			cpuMax := readMax(path)
			if cpuMax.Err == nil || !strings.Contains(cpuMax.Err.Error(), testCase.expect) {
				t.Fatalf("expected error containing %q, got %+v", testCase.expect, cpuMax)
			}
		})
	}
}

func TestReadSelfCgroupSkipsInvalidLines(t *testing.T) {
	t.Parallel()

	procPath := filepath.Join(t.TempDir(), "proc")

	content := strings.Join([]string{
		"invalid",
		"1:name=systemd:/ignored", // controller entry should be skipped
		"2::",                     // missing path should be skipped
	}, "\n")

	err := os.WriteFile(procPath, []byte(content+"\n"), 0o600)
	if err != nil {
		t.Fatalf("write cgroup file: %v", err)
	}

	_, err = readSelfCgroup(procPath)
	if !errors.Is(err, errCgroupPathNotFound) {
		t.Fatalf("expected errCgroupPathNotFound, got %v", err)
	}
}

func TestReadSelfCgroupHandlesMissingControllerEntries(t *testing.T) {
	t.Parallel()

	procPath := filepath.Join(t.TempDir(), "proc")

	content := strings.Join([]string{
		"not-even-close",              // malformed without expected parts
		"0:cpu:/user.slice",           // controller present, should be skipped
		"1:name=systemd:/slice.scope", // controllers populated, should be skipped
		"2::   ",                      // missing path, should be skipped
	}, "\n")

	err := os.WriteFile(procPath, []byte(content+"\n"), 0o600)
	if err != nil {
		t.Fatalf("write cgroup file: %v", err)
	}

	_, err = readSelfCgroup(procPath)
	if !errors.Is(err, errCgroupPathNotFound) {
		t.Fatalf("expected errCgroupPathNotFound, got %v", err)
	}
}

func TestReadSelfCgroupPropagatesScannerError(t *testing.T) {
	t.Parallel()

	procPath := filepath.Join(t.TempDir(), "proc")

	longLine := strings.Repeat("x", bufio.MaxScanTokenSize+32)

	err := os.WriteFile(procPath, []byte(longLine), 0o600)
	if err != nil {
		t.Fatalf("write oversized cgroup file: %v", err)
	}

	_, err = readSelfCgroup(procPath)
	if err == nil {
		t.Fatalf("expected scan error, got nil")
	}

	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("expected bufio.ErrTooLong, got %v", err)
	}

	if !strings.Contains(err.Error(), "scan "+procPath) {
		t.Fatalf("expected scan path prefix, got %v", err)
	}
}

func TestBuildControllerPathNormalisesInputs(t *testing.T) {
	t.Parallel()

	custom := buildControllerPath("/root/", "/slice.scope/", "cpu.weight")

	expected := filepath.Join("/root", "slice.scope", "cpu.weight")
	if custom != expected {
		t.Fatalf("expected %q, got %q", expected, custom)
	}

	defaulted := buildControllerPath("", "/tenant.slice", "cpu.max")

	defaultExpected := filepath.Join(defaultCgroupRoot, "tenant.slice", "cpu.max")
	if defaulted != defaultExpected {
		t.Fatalf("expected default root %q, got %q", defaultExpected, defaulted)
	}
}
