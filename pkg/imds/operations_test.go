package imds_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/imds"
)

func TestHTTPClientHappyPath(t *testing.T) {
	t.Parallel()

	region := "us-phoenix-1\n"
	canonicalRegion := "us-phoenix-1 "
	instanceID := "ocid1.instance.oc1..exampleuniqueID"
	compartmentID := "ocid1.compartment.oc1..exampleCompartment"
	shapeBody := `{"ocpus":4,"memoryInGBs":64,` +
		`"baselineOcpuUtilization":"BASELINE_1_1","baselineOcpus":4,` +
		`"threadsPerCore":2,"networkingBandwidthInGbps":10,"maxVnicAttachments":2}`
	instanceBody := `{"regionInfo":{"canonicalRegionName":"` + canonicalRegion + `","regionIdentifier":"phx"}}`

	responses := map[string]string{
		regionResourcePath:          region,
		canonicalRegionResourcePath: instanceBody,
		instanceIDResourcePath:      instanceID,
		compartmentIDResourcePath:   compartmentID,
		shapeConfigResourcePath:     shapeBody,
	}

	client := newIMDSTestClient(t, responses)

	ctx := context.Background()

	gotRegion, err := client.Region(ctx)
	requireNoError(t, err, "Region()")
	requireEqual(t, "Region()", gotRegion, "us-phoenix-1")

	gotCanonicalRegion, err := client.CanonicalRegion(ctx)
	requireNoError(t, err, "CanonicalRegion()")
	requireEqual(t, "CanonicalRegion()", gotCanonicalRegion, "us-phoenix-1")

	gotID, err := client.InstanceID(ctx)
	requireNoError(t, err, "InstanceID()")
	requireEqual(t, "InstanceID()", gotID, instanceID)

	gotCompartmentID, err := client.CompartmentID(ctx)
	requireNoError(t, err, "CompartmentID()")
	requireEqual(t, "CompartmentID()", gotCompartmentID, compartmentID)

	shapeCfg, err := client.ShapeConfig(ctx)
	requireNoError(t, err, "ShapeConfig()")

	requireEqual(t, "ShapeConfig().OCPUs", shapeCfg.OCPUs, 4.0)
	requireEqual(t, "ShapeConfig().MemoryInGBs", shapeCfg.MemoryInGBs, 64.0)
	requireEqual(t, "ShapeConfig().MaxVnicAttachments", shapeCfg.MaxVnicAttachments, 2)
}

func TestShapeConfigDecodeError(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			if req.URL.Path != shapeConfigResourcePath {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			requireIMDSAuthHeader(t, req)

			_, _ = writer.Write([]byte("not-json"))
		}),
	)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(httpClient, imds.WithBaseURL(server.URL+"/opc/v2"))

	_, err := client.ShapeConfig(context.Background())
	if err == nil {
		t.Fatal("ShapeConfig() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "decode shapeConfig response") {
		t.Fatalf("ShapeConfig() error = %v, want decode failure", err)
	}
}

func TestHTTPClientEmptyResponses(t *testing.T) {
	t.Parallel()

	responses := map[string]string{
		regionResourcePath:          "  \t\n",
		canonicalRegionResourcePath: `{"regionInfo":{"canonicalRegionName":"   "}}`,
		instanceIDResourcePath:      "",
		compartmentIDResourcePath:   " ",
	}

	client := newIMDSTestClient(t, responses)

	ctx := context.Background()

	_, err := client.Region(ctx)
	requireErrorContains(t, "Region()", err, "empty response")

	_, err = client.CanonicalRegion(ctx)
	requireErrorContains(t, "CanonicalRegion()", err, "canonicalRegionName")

	_, err = client.InstanceID(ctx)
	requireErrorContains(t, "InstanceID()", err, "empty response")

	_, err = client.CompartmentID(ctx)
	requireErrorContains(t, "CompartmentID()", err, "empty response")
}
