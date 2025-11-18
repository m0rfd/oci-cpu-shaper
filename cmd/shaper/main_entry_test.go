package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMainSuccessDoesNotExit(t *testing.T) { //nolint:paralleltest // mutates process-wide state
	originalExit := exitProcess

	defer func() { exitProcess = originalExit }()

	exitCalled := false
	exitProcess = func(code int) {
		exitCalled = true

		if code != exitCodeSuccess {
			t.Fatalf("unexpected exit code: %d", code)
		}
	}

	originalArgs := os.Args

	defer func() { os.Args = originalArgs }()

	os.Args = []string{"oci-cpu-shaper", "--mode", "noop"}

	main()

	if exitCalled {
		t.Fatal("expected main to complete without invoking exit")
	}
}

func TestMainPropagatesNonZeroExitCode(t *testing.T) { //nolint:paralleltest // mutates global state
	originalExit := exitProcess

	defer func() { exitProcess = originalExit }()

	exitCodes := make(chan int, 1)
	exitProcess = func(code int) {
		exitCodes <- code
	}

	originalArgs := os.Args

	defer func() { os.Args = originalArgs }()

	os.Args = []string{"oci-cpu-shaper", "--mode", "invalid"}

	main()

	select {
	case code := <-exitCodes:
		if code != exitCodeParseError {
			t.Fatalf("expected exit code %d, got %d", exitCodeParseError, code)
		}
	default:
		t.Fatal("expected main to invoke exit with parse error code")
	}
}

func TestMainIntegratesDefaultDependencies(t *testing.T) {
	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			if req.Header.Get(imdsAuthHeaderKey) != imdsAuthHeaderVal {
				t.Fatalf(
					"expected IMDS authorization header %q, got %q",
					imdsAuthHeaderVal,
					req.Header.Get(imdsAuthHeaderKey),
				)
			}

			switch req.URL.Path {
			case "/opc/v2/instance/region":
				_, _ = writer.Write([]byte("us-denver-1"))
			case "/opc/v2/instance/", "/opc/v2/instance":
				_, _ = writer.Write([]byte(`{"regionInfo":{"canonicalRegionName":"us-denver-1"}}`))
			case "/opc/v2/instance/id":
				_, _ = writer.Write([]byte("ocid1.instance.oc1..main"))
			case "/opc/v2/instance/compartmentId":
				_, _ = writer.Write([]byte("ocid1.compartment.oc1..main"))
			case "/opc/v2/instance/shapeConfig":
				_, _ = writer.Write([]byte(`{"ocpus":1,"memoryInGBs":1}`))
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
		}),
	)
	t.Cleanup(server.Close)

	t.Setenv(imdsEndpointEnv, server.URL+"/opc/v2")

	originalArgs := os.Args
	os.Args = []string{
		"shaper",
		"--mode",
		"noop",
		"--log-level",
		"error",
		"--config",
		"./testdata/config.yaml",
	}

	defer func() { os.Args = originalArgs }()

	done := make(chan struct{})

	go func() {
		defer close(done)

		main()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("main did not return in time")
	}
}
