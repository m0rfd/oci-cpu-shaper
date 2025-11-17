// Package cgroup inspects Linux cgroup v2 CPU controls exposed to the shaper.
package cgroup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultProcSelfCgroup = "/proc/self/cgroup"
	defaultCgroupRoot     = "/sys/fs/cgroup"

	cpuWeightFile = "cpu.weight"
	cpuMaxFile    = "cpu.max"

	cgroupLineParts  = 3
	cpuMaxFieldParts = 2
)

var (
	errCgroupPathNotFound = errors.New("cgroup: cpu controller path not found")
	errEmptyValue         = errors.New("cgroup: empty value")
	errExpectedTwoFields  = errors.New("cgroup: expected two fields")
)

// Reader discovers and parses CPU controller settings for the running process.
type Reader struct {
	// ProcPath points to the procfs file that lists the current cgroup paths.
	ProcPath string
	// RootPath is the filesystem root containing the cgroup v2 controller files.
	RootPath string
}

// CPU captures cpu.weight and cpu.max values alongside any parse errors.
type CPU struct {
	Path   string
	Weight Weight
	Max    Max
}

// Weight describes a parsed cpu.weight value.
type Weight struct {
	Path      string
	Value     uint64
	Available bool
	Err       error
}

// Max describes a parsed cpu.max quota/period tuple.
type Max struct {
	Path      string
	Quota     uint64
	Period    uint64
	Unlimited bool
	Available bool
	Err       error
}

// Detect returns the current CPU controller configuration for the running process.
// Missing cpu.weight or cpu.max files are reported through the Weight/Max error fields
// while still returning a CPU struct so callers can continue with degraded telemetry.
func (r Reader) Detect() (*CPU, error) {
	procPath := r.ProcPath
	if strings.TrimSpace(procPath) == "" {
		procPath = defaultProcSelfCgroup
	}

	rootPath := r.RootPath
	if strings.TrimSpace(rootPath) == "" {
		rootPath = defaultCgroupRoot
	}

	cgroupPath, err := readSelfCgroup(procPath)
	if err != nil {
		return nil, err
	}

	cpuInfo := new(CPU)
	cpuInfo.Path = cgroupPath

	weightPath := buildControllerPath(rootPath, cgroupPath, cpuWeightFile)
	cpuInfo.Weight = readWeight(weightPath)

	maxPath := buildControllerPath(rootPath, cgroupPath, cpuMaxFile)
	cpuInfo.Max = readMax(maxPath)

	return cpuInfo, nil
}

func readSelfCgroup(procPath string) (string, error) {
	file, err := os.Open(procPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", procPath, err)
	}

	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", cgroupLineParts)
		if len(parts) != cgroupLineParts {
			continue
		}

		controllers := strings.TrimSpace(parts[1])
		if controllers != "" {
			continue
		}

		path := strings.TrimSpace(parts[2])
		if path == "" {
			continue
		}

		return path, nil
	}

	scanErr := scanner.Err()
	if scanErr != nil {
		return "", fmt.Errorf("scan %s: %w", procPath, scanErr)
	}

	return "", errCgroupPathNotFound
}

func buildControllerPath(root, relPath, filename string) string {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		trimmedRoot = defaultCgroupRoot
	}

	rel := strings.TrimSpace(relPath)
	rel = strings.TrimPrefix(rel, "/")

	full := filepath.Join(trimmedRoot, rel, filename)

	return full
}

func readWeight(path string) Weight {
	data, err := os.ReadFile(path)
	if err != nil {
		return Weight{
			Path:      path,
			Value:     0,
			Available: false,
			Err:       fmt.Errorf("read %s: %w", path, err),
		}
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return Weight{
			Path:      path,
			Value:     0,
			Available: false,
			Err:       fmt.Errorf("parse %s: %w", path, errEmptyValue),
		}
	}

	value, parseErr := strconv.ParseUint(trimmed, 10, 64)
	if parseErr != nil {
		return Weight{
			Path:      path,
			Value:     0,
			Available: false,
			Err:       fmt.Errorf("parse %s: %w", path, parseErr),
		}
	}

	return Weight{Path: path, Value: value, Available: true, Err: nil}
}

func readMax(path string) Max {
	data, err := os.ReadFile(path)
	if err != nil {
		return newMaxError(path, fmt.Errorf("read %s: %w", path, err))
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return newMaxError(path, fmt.Errorf("parse %s: %w", path, errEmptyValue))
	}

	fields := strings.Fields(trimmed)
	if len(fields) != cpuMaxFieldParts {
		return newMaxError(path, fmt.Errorf("parse %s: %w", path, errExpectedTwoFields))
	}

	period, perErr := strconv.ParseUint(fields[1], 10, 64)
	if perErr != nil {
		return newMaxError(path, fmt.Errorf("parse %s period: %w", path, perErr))
	}

	if strings.EqualFold(fields[0], "max") {
		return Max{
			Path:      path,
			Quota:     0,
			Period:    period,
			Unlimited: true,
			Available: true,
			Err:       nil,
		}
	}

	quota, quotaErr := strconv.ParseUint(fields[0], 10, 64)
	if quotaErr != nil {
		return newMaxError(path, fmt.Errorf("parse %s quota: %w", path, quotaErr))
	}

	return Max{
		Path:      path,
		Quota:     quota,
		Period:    period,
		Unlimited: false,
		Available: true,
		Err:       nil,
	}
}

func newMaxError(path string, err error) Max {
	return Max{
		Path:      path,
		Quota:     0,
		Period:    0,
		Unlimited: false,
		Available: false,
		Err:       err,
	}
}
