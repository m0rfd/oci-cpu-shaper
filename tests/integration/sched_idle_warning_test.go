//go:build integration && linux

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	schedIdleWarningMessage = "worker failed to enter sched_idle"
	modeBBaseImage          = "gcr.io/distroless/static:latest"
	modeBConfigFilename     = "mode-b.yaml"
	rootfulBinaryName       = "oci-cpu-shaper-rootful"
	rootUser                = "0:0"
	nobodyUser              = "65534:65534"
)

func TestSchedIdleWarningTracksSysNiceCapability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	requireDocker(t)

	repoRoot := repositoryRoot(t)
	assetsDir := t.TempDir()
	if err := os.Chmod(assetsDir, 0o755); err != nil {
		t.Fatalf("chmod assets dir: %v", err)
	}

	binaryPath := buildRootfulShaperBinary(t, repoRoot, assetsDir)
	configPath := copyModeBConfig(t, repoRoot, assetsDir)

	missingCapLogs := runModeBContainer(t, schedIdleContainerConfig{
		assetsDir:      assetsDir,
		binaryPath:     binaryPath,
		configFilename: filepath.Base(configPath),
		user:           nobodyUser,
	})

	if !strings.Contains(missingCapLogs, schedIdleWarningMessage) {
		t.Fatalf("expected sched_idle warning when SYS_NICE is missing. container logs:\n%s", missingCapLogs)
	}

	presentCapLogs := runModeBContainer(t, schedIdleContainerConfig{
		assetsDir:      assetsDir,
		binaryPath:     binaryPath,
		configFilename: filepath.Base(configPath),
		user:           rootUser,
		addSysNice:     true,
	})

	if strings.Contains(presentCapLogs, schedIdleWarningMessage) {
		t.Fatalf("did not expect sched_idle warning when SYS_NICE is present. container logs:\n%s", presentCapLogs)
	}
}

func buildRootfulShaperBinary(t *testing.T, repoRoot, dstDir string) string {
	t.Helper()

	binaryPath := filepath.Join(dstDir, rootfulBinaryName)

	cmd := exec.Command(
		"go", "build",
		"-tags", "rootful",
		"-o", binaryPath,
		"./cmd/shaper",
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build rootful shaper binary: %v\n%s", err, output)
	}

	return binaryPath
}

func copyModeBConfig(t *testing.T, repoRoot, dstDir string) string {
	t.Helper()

	src := filepath.Join(repoRoot, "configs", modeBConfigFilename)
	dst := filepath.Join(dstDir, modeBConfigFilename)

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read mode-b config: %v", err)
	}

	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write mode-b config: %v", err)
	}

	return dst
}

type schedIdleContainerConfig struct {
	assetsDir      string
	binaryPath     string
	configFilename string
	user           string
	dropSysNice    bool
	addSysNice     bool
}

func runModeBContainer(t *testing.T, cfg schedIdleContainerConfig) string {
	t.Helper()

	containerArgs := []string{
		"run",
		"--rm",
		"-e", "OCI_OFFLINE=true",
		"-v", fmt.Sprintf("%s:/workspace:ro", cfg.assetsDir),
		"--entrypoint", fmt.Sprintf("/workspace/%s", filepath.Base(cfg.binaryPath)),
	}

	if cfg.user != "" {
		containerArgs = append(containerArgs, "--user", cfg.user)
	}

	if cfg.dropSysNice {
		containerArgs = append(containerArgs, "--cap-drop", "SYS_NICE")
	}

	if cfg.addSysNice {
		containerArgs = append(containerArgs, "--cap-add", "SYS_NICE")
	}

	containerArgs = append(containerArgs,
		modeBBaseImage,
		"--config", fmt.Sprintf("/workspace/%s", cfg.configFilename),
		"--mode", "dry-run",
		"--log-level", "info",
		"--shutdown-after", "5s",
	)

	cmd := exec.Command("docker", containerArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run mode-b container: %v\n%s", err, output)
	}

	return string(output)
}

func requireDocker(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker CLI not available: %v", err)
	}
}
