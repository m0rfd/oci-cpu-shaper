//go:build integration

// Package integration exercises container-level responsiveness guarantees.
package integration

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	tc "github.com/testcontainers/testcontainers-go"
)

const (
	integrationRootfulImageTag  = "oci-cpu-shaper:integration-rootful"
	integrationRootlessImageTag = "oci-cpu-shaper:integration-rootless"
	hogCmdImportPath            = "./tests/integration/cmd/cpu-hog"
)

func TestCPUWeightResponsiveness(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("integration test requires a Linux host")
	}

	ensureCgroupV2(t)

	dockerClient := dockerAPI(t)
	ctx := t.Context()

	repoRoot := repositoryRoot(t)
	hogBinary := buildHogBinary(t, repoRoot)
	buildIntegrationImage(t, repoRoot, "rootful", integrationRootfulImageTag)
	buildIntegrationImage(t, repoRoot, "rootless", integrationRootlessImageTag)

	variants := []struct {
		name  string
		image string
	}{
		{name: "rootful", image: integrationRootfulImageTag},
		{name: "rootless", image: integrationRootlessImageTag},
	}

	for _, variant := range variants {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			assertCPUWeightRatio(t, ctx, dockerClient, hogBinary, variant.name, variant.image)
		})
	}
}

func assertCPUWeightRatio(t *testing.T, ctx context.Context, dockerClient *client.Client, hogBinary, variantName, lowWeightImage string) {
	helperSuffix := fmt.Sprintf("cpu-weight-high-%s", variantName)
	highWeightName := containerName(helperSuffix)
	lowWeightName := containerName(fmt.Sprintf("cpu-weight-low-%s", variantName))

	highWeight := runContainer(t, ctx, dockerClient, containerConfig{
		name:       highWeightName,
		image:      "alpine:3.20",
		cpuShares:  1024,
		hogBinary:  hogBinary,
		duration:   45 * time.Second,
		cpuWorkers: 1,
	})
	lowWeight := runContainer(t, ctx, dockerClient, containerConfig{
		name:       lowWeightName,
		image:      lowWeightImage,
		cpuShares:  2,
		hogBinary:  hogBinary,
		duration:   45 * time.Second,
		cpuWorkers: 1,
	})

	time.Sleep(10 * time.Second)

	highWeightStats := readCPUStats(t, ctx, dockerClient, highWeight)
	lowWeightStats := readCPUStats(t, ctx, dockerClient, lowWeight)

	t.Logf("[%s] high-weight container usage: %d µs (weight=%d)", variantName, highWeightStats.usageMicros, highWeightStats.weight)
	t.Logf("[%s] low-weight container usage: %d µs (weight=%d)", variantName, lowWeightStats.usageMicros, lowWeightStats.weight)

	if highWeightStats.weight <= lowWeightStats.weight {
		t.Fatalf("[%s] expected high-weight container (%d) to exceed low-weight container (%d)", variantName, highWeightStats.weight, lowWeightStats.weight)
	}

	if lowWeightStats.usageMicros == 0 {
		t.Fatalf("[%s] low-weight container reported zero CPU usage; inspect docker logs for %s", variantName, lowWeightName)
	}

	usageRatio := float64(highWeightStats.usageMicros) / float64(lowWeightStats.usageMicros)
	t.Logf("[%s] observed CPU usage ratio (high/low): %.2f", variantName, usageRatio)

	const minimumExpectedRatio = 5.0
	if usageRatio < minimumExpectedRatio {
		t.Fatalf("[%s] expected high-weight container to receive at least %.1fx CPU time (got %.2fx)", variantName, minimumExpectedRatio, usageRatio)
	}
}

type containerConfig struct {
	name       string
	image      string
	cpuShares  int
	hogBinary  string
	duration   time.Duration
	cpuWorkers int
}

type cpuStats struct {
	usageMicros uint64
	weight      uint64
}

func ensureCgroupV2(t *testing.T) {
	t.Helper()

	controllersPath := "/sys/fs/cgroup/cgroup.controllers"
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		t.Fatalf("cgroup v2 controllers file not readable (%s): %v", controllersPath, err)
	}

	if !strings.Contains(string(data), "cpu") {
		t.Fatalf("cgroup v2 cpu controller is unavailable; controllers=%q", strings.TrimSpace(string(data)))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("determine working directory: %v", err)
	}

	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err = os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	return root
}

func buildHogBinary(t *testing.T, repoRoot string) string {
	t.Helper()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "cpu-hog")

	build := exec.Command("go", "build", "-o", binaryPath, hogCmdImportPath)
	build.Dir = repoRoot
	build.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)

	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cpu hog helper: %v\n%s", err, output)
	}

	return binaryPath
}

func buildIntegrationImage(t *testing.T, repoRoot, target, tag string) {
	t.Helper()

	repo := tag
	imageTag := ""
	if splitRepo, splitTag, ok := strings.Cut(tag, ":"); ok {
		repo = splitRepo
		imageTag = splitTag
	}

	provider, err := tc.NewDockerProvider()
	if err != nil {
		t.Fatalf("create docker provider: %v", err)
	}
	defer provider.Close()

	ctx := t.Context()

	_, err = provider.BuildImage(ctx, &tc.ContainerRequest{ //nolint:contextcheck
		FromDockerfile: tc.FromDockerfile{
			Context:       repoRoot,
			Dockerfile:    "Dockerfile",
			Repo:          repo,
			Tag:           imageTag,
			PrintBuildLog: true,
			KeepImage:     true,
			BuildOptionsModifier: func(options *types.ImageBuildOptions) {
				options.Target = target
			},
		},
	})
	if err != nil {
		t.Fatalf("build integration image (target=%s tag=%s): %v", target, tag, err)
	}
}

func runContainer(t *testing.T, ctx context.Context, dockerClient *client.Client, cfg containerConfig) tc.Container {
	t.Helper()

	request := tc.ContainerRequest{
		Name:       cfg.name,
		Image:      cfg.image,
		Entrypoint: []string{"/hog"},
		Cmd: []string{
			fmt.Sprintf("-duration=%ds", int(cfg.duration.Seconds())),
			fmt.Sprintf("-workers=%d", cfg.cpuWorkers),
		},
		SkipReaper: true,
		HostConfigModifier: func(config *container.HostConfig) {
			config.CPUShares = int64(cfg.cpuShares)
			config.CpusetCpus = "0"
			config.Binds = append(config.Binds, fmt.Sprintf("%s:/hog:ro", cfg.hogBinary))
		},
	}

	cont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: request, Started: true})
	if err != nil {
		t.Fatalf("start container %s: %v", cfg.name, err)
	}

	waitForRunning(t, ctx, dockerClient, cfg.name, cont, 10*time.Second)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = cont.Terminate(cleanupCtx) // best effort cleanup
	})

	return cont
}

func waitForRunning(t *testing.T, ctx context.Context, dockerClient *client.Client, name string, cont tc.Container, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		inspect, err := dockerClient.ContainerInspect(ctx, cont.GetContainerID())
		if err == nil && inspect.State != nil && inspect.State.Running {
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("container %s did not report running state within %s", name, timeout)
}

func readCPUStats(t *testing.T, ctx context.Context, dockerClient *client.Client, cont tc.Container) cpuStats {
	t.Helper()

	pid := containerPID(t, ctx, dockerClient, cont)
	cgroupPath := cgroupPathForPID(t, pid)

	statsPath := filepath.Join(cgroupPath, "cpu.stat")
	weightPath := filepath.Join(cgroupPath, "cpu.weight")

	usage, err := parseUsageMicros(statsPath)
	if err != nil {
		t.Fatalf("parse cpu.stat for %s: %v", contName(cont), err)
	}

	weight, err := parseWeight(weightPath)
	if err != nil {
		t.Fatalf("parse cpu.weight for %s: %v", contName(cont), err)
	}

	return cpuStats{
		usageMicros: usage,
		weight:      weight,
	}
}

func containerPID(t *testing.T, ctx context.Context, dockerClient *client.Client, cont tc.Container) int { //nolint:unparam // container pid read is consistent
	t.Helper()

	inspect, err := dockerClient.ContainerInspect(ctx, cont.GetContainerID())
	if err != nil {
		t.Fatalf("inspect container %s pid: %v", contName(cont), err)
	}

	if inspect.State == nil {
		t.Fatalf("container %s missing state", contName(cont))
	}

	pid := inspect.State.Pid
	if pid <= 0 {
		t.Fatalf("container %s reported invalid pid %d", contName(cont), pid)
	}

	return pid
}

func cgroupPathForPID(t *testing.T, pid int) string {
	t.Helper()

	cgroupFile := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupFile)
	if err != nil {
		t.Fatalf("read cgroup data for pid %d: %v", pid, err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			continue
		}

		if parts[0] == "0" {
			relPath := parts[2]
			if relPath == "" {
				break
			}

			absPath := filepath.Join("/sys/fs/cgroup", relPath)
			if _, err := os.Stat(absPath); err == nil {
				return absPath
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan cgroup entries for pid %d: %v", pid, err)
	}

	t.Fatalf("cgroup v2 path not found for pid %d", pid)

	return ""
}

func parseUsageMicros(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "usage_usec" {
			value, convErr := strconv.ParseUint(fields[1], 10, 64)
			if convErr != nil {
				return 0, convErr
			}

			return value, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("usage_usec not present in cpu.stat")
}

func parseWeight(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, errors.New("cpu.weight empty")
	}

	return strconv.ParseUint(trimmed, 10, 64)
}

func containerName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func dockerAPI(t *testing.T) *client.Client {
	t.Helper()

	clientOpts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		t.Fatalf("connect to docker api: %v", err)
	}

	t.Cleanup(func() {
		_ = cli.Close()
	})

	return cli
}

func contName(cont tc.Container) string {
	name, err := cont.Name(context.Background())
	if err != nil {
		return cont.GetContainerID()
	}

	return strings.TrimPrefix(name, "/")
}
