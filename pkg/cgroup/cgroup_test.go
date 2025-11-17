package cgroup_test

import (
	"os"
	"path/filepath"
	"testing"

	"oci-cpu-shaper/pkg/cgroup"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()

	err := os.MkdirAll(path, 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReaderDetectsCPUWeightAndMax(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "cgroup")
	mkdirAll(t, rootPath)

	relPath := "/user.slice/test.scope"
	controllerDir := filepath.Join(rootPath, "user.slice", "test.scope")
	mkdirAll(t, controllerDir)

	writeFile(t, filepath.Join(controllerDir, "cpu.weight"), "256\n")
	writeFile(t, filepath.Join(controllerDir, "cpu.max"), "50000 100000\n")
	procPath := filepath.Join(tmpDir, "proc")
	writeFile(t, procPath, "0::"+relPath+"\n")

	reader := cgroup.Reader{ProcPath: procPath, RootPath: rootPath}

	info, err := reader.Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if info.Path != relPath {
		t.Fatalf("expected path %q, got %q", relPath, info.Path)
	}

	if !info.Weight.Available {
		t.Fatalf("expected weight to be available: %+v", info.Weight)
	}

	if info.Weight.Value != 256 {
		t.Fatalf("expected weight 256, got %d", info.Weight.Value)
	}

	if info.Max.Unlimited {
		t.Fatal("expected finite cpu.max quota")
	}

	if info.Max.Quota != 50000 || info.Max.Period != 100000 {
		t.Fatalf("unexpected max values: %+v", info.Max)
	}
}

func TestReaderDetectsUnlimitedCPUMax(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root")
	slicePath := filepath.Join(rootPath, "slice")
	mkdirAll(t, slicePath)

	writeFile(t, filepath.Join(slicePath, "cpu.weight"), "100\n")
	writeFile(t, filepath.Join(slicePath, "cpu.max"), "max 100000")
	procPath := filepath.Join(tmpDir, "proc")
	writeFile(t, procPath, "0::/slice\n")

	reader := cgroup.Reader{ProcPath: procPath, RootPath: rootPath}

	info, err := reader.Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if !info.Max.Unlimited {
		t.Fatalf("expected unlimited max, got %+v", info.Max)
	}

	if info.Max.Period != 100000 {
		t.Fatalf("expected period 100000, got %d", info.Max.Period)
	}
}

func TestReaderReturnsErrorsWhenFilesMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root")
	mkdirAll(t, filepath.Join(rootPath, "slice"))

	procPath := filepath.Join(tmpDir, "proc")
	writeFile(t, procPath, "0::/slice\n")

	reader := cgroup.Reader{ProcPath: procPath, RootPath: rootPath}

	info, err := reader.Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if info.Weight.Err == nil {
		t.Fatalf("expected weight error, got %+v", info.Weight)
	}

	if info.Max.Err == nil {
		t.Fatalf("expected max error when file missing")
	}
}

func TestReaderFailsWhenCgroupPathMissing(t *testing.T) {
	t.Parallel()

	procPath := filepath.Join(t.TempDir(), "proc")
	writeFile(t, procPath, "")

	reader := cgroup.Reader{ProcPath: procPath, RootPath: t.TempDir()}

	_, err := reader.Detect()
	if err == nil {
		t.Fatal("expected error when cgroup path not found")
	}
}
