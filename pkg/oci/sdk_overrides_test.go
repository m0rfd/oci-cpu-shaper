package oci //nolint:testpackage

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
)

func stubConfigurationProvider(t *testing.T) fakeConfigurationProvider {
	t.Helper()

	key := testPrivateKey(t)

	return fakeConfigurationProvider{key: key}
}

func testPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test RSA key: %v", err)
	}

	return key
}

type fakeConfigurationProvider struct {
	key *rsa.PrivateKey
}

func (f fakeConfigurationProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	return f.key, nil
}

func (f fakeConfigurationProvider) KeyID() (string, error) {
	return "ocid1.tenancy.oc1..test/ocid1.user.oc1..test/fingerprint", nil
}

func (f fakeConfigurationProvider) TenancyOCID() (string, error) {
	return "ocid1.tenancy.oc1..test", nil
}

func (f fakeConfigurationProvider) UserOCID() (string, error) {
	return "ocid1.user.oc1..test", nil
}

func (f fakeConfigurationProvider) KeyFingerprint() (string, error) {
	return "fingerprint", nil
}

func (f fakeConfigurationProvider) Region() (string, error) {
	return "us-phoenix-1", nil
}

func (f fakeConfigurationProvider) AuthType() (common.AuthConfig, error) {
	return common.AuthConfig{
		AuthType:         common.AuthenticationType("instance_principal"),
		IsFromConfigFile: false,
		OboToken:         nil,
	}, nil
}

type stubAPICaller struct {
	response    *http.Response
	err         error
	lastRequest *http.Request
}

func newStubAPICaller(response *http.Response, err error) *stubAPICaller {
	return &stubAPICaller{response: response, err: err, lastRequest: nil}
}

func (s *stubAPICaller) Call(
	_ context.Context,
	req *http.Request,
) (*http.Response, error) {
	s.lastRequest = req

	if s.err != nil {
		return nil, s.err
	}

	return s.response, nil
}
