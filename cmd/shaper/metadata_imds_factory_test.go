package main

import (
	"context"
	"net/http"
	"testing"
)

func TestDefaultIMDSFactoryUsesEnvironmentEndpoint(t *testing.T) {
	responses := map[string]string{
		"/opc/v2/instance/region":      overrideRegion,
		"/opc/v2/instance/id":          "ocid1.instance.oc1..exampleuniqueID",
		"/opc/v2/instance/shapeConfig": `{"ocpus":2,"memoryInGBs":32}`,
	}

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			body, ok := responses[req.URL.Path]
			if !ok {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			_, _ = writer.Write([]byte(body))
		}),
	)
	t.Cleanup(server.Close)

	t.Setenv(imdsEndpointEnv, " "+server.URL+"/opc/v2 ")

	client := defaultIMDSFactory()

	ctx := context.Background()

	region, err := client.Region(ctx)
	if err != nil {
		t.Fatalf("Region() returned error: %v", err)
	}

	if region != overrideRegion {
		t.Fatalf("unexpected region %q", region)
	}

	instanceID, err := client.InstanceID(ctx)
	if err != nil {
		t.Fatalf("InstanceID() returned error: %v", err)
	}

	if instanceID != "ocid1.instance.oc1..exampleuniqueID" {
		t.Fatalf("unexpected instance ID %q", instanceID)
	}

	shape, err := client.ShapeConfig(ctx)
	if err != nil {
		t.Fatalf("ShapeConfig() returned error: %v", err)
	}

	if shape.OCPUs != 2 || shape.MemoryInGBs != 32 {
		t.Fatalf("unexpected shape config: %+v", shape)
	}
}
