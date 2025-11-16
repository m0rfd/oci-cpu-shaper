package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	requiredArgs    = 2
	usageExitCode   = 2
	minCgroupFields = 3
)

var errRootCgroupMissing = errors.New("root cgroup entry not found")

func main() {
	if len(os.Args) < requiredArgs {
		fmt.Fprintln(os.Stderr, "usage: cgroup-reader <path|stat|weight|max>")
		os.Exit(usageExitCode)
	}

	var err error

	switch os.Args[1] {
	case "path":
		err = printCgroupPath()
	case "stat":
		err = outputFile(os.Stdout, "/sys/fs/cgroup/cpu.stat")
	case "weight":
		err = outputFile(os.Stdout, "/sys/fs/cgroup/cpu.weight")
	case "max":
		err = outputFile(os.Stdout, "/sys/fs/cgroup/cpu.max")
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		os.Exit(usageExitCode)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printCgroupPath() error {
	rel, err := rootCgroupPathFromFile("/proc/self/cgroup")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(os.Stdout, rel)
	if err != nil {
		return fmt.Errorf("write rel path: %w", err)
	}

	return nil
}

func rootCgroupPathFromFile(path string) (string, error) {
	fileHandle, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}

	defer func() {
		_ = fileHandle.Close()
	}()

	rel, err := rootCgroupPath(fileHandle)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	return rel, nil
}

func rootCgroupPath(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < minCgroupFields {
			continue
		}

		if parts[0] == "0" {
			return parts[2], nil
		}
	}

	err := scanner.Err()
	if err != nil {
		return "", fmt.Errorf("scan cgroup data: %w", err)
	}

	return "", errRootCgroupMissing
}

func outputFile(dst io.Writer, path string) error {
	fileHandle, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	defer func() {
		_ = fileHandle.Close()
	}()

	_, err = io.Copy(dst, fileHandle)
	if err != nil {
		return fmt.Errorf("copy %s: %w", path, err)
	}

	return nil
}
