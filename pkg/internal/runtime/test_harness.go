/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime

// This file exposes a test harness for the runtime.

import (
	"context"
	"math/rand/v2"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-test/v5/driver"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	utiltesting "k8s.io/client-go/util/testing"
)

// TestHarness provides a Runtime that can either work with a fake CSI driver
// based on the csi-test framework, or a mock CSI driver with the CSI Identity and
// CSI SnapshotMetadata servers.
//
//  1. Mock CSI driver usage example
//     th := NewTestHarness().WithMockCSIDriver(t)
//     defer th.TerminateMockCSIDriver()
//     th.MockCSIIdentityServer.EXPECT().GetDriverName(gomock.Any(), gomock.Any()).Return("driver",nil)
//     rt := th.RuntimeForMockCSIDriver(t)
//     name, err := csirpc.GetDriverName(ctx, rt.CSIConn) // sample GRPC client call
//
//  2. Fake CSI driver usage example
//     sms := &fakeSnapshotMetadataServer{} // optional
//     th := NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, sms)
//     defer th.RemoveFakeKubeConfig(t)
//     defer th.TerminateFakeCSIDriver(t)
//     rt := th.RuntimeForFakeCSIDriver(t)
//     name, err := csirpc.GetDriverName(ctx, rt.CSIConn) // sample GRPC client call
//
//  3. The runtime args can point to real TLS cert and key files instead of non-existent paths.
//     th := NewTestHarness().WithTestTLSFiles(t).With...
//     defer th.RemoveFakeTLSFiles(t)
//     rta := th.RuntimeArgs()
//     cert, err := tls.LoadX509KeyPair(rta.TLSCertFile, rta.TLSKeyFile)
type TestHarness struct {
	MockController                *gomock.Controller
	MockCSIDriver                 *driver.MockCSIDriver
	MockCSIIdentityServer         *driver.MockIdentityServer
	MockCSISnapshotMetadataServer *driver.MockSnapshotMetadataServer

	FakeCSIDriver *driver.CSIDriver

	// Identity server responses
	FakeCSIDriverName                 string
	FakeProbeResponse                 *csi.ProbeResponse
	FakeGetPluginCapabilitiesResponse *csi.GetPluginCapabilitiesResponse

	// internal
	fakeCSIDriverAddr  string
	fakeKubeConfigFile *os.File
	fakeGRPCServerDir  string
	mockCSIDriverConn  *grpc.ClientConn
	tlsCertFile        string
	tlsKeyFile         string
	tlsGenerator       *testTLSCertGenerator
	// Minimize port-in-use errors from back-to-back test harness usage
	// by dynamically assigning a port number to be used in the runtime
	// arguments and ensuring that numbers are not repeated (at least not
	// in the same process).
	rtaPortNumber int

	// for the mock/fake servers
	*csi.UnimplementedIdentityServer
}

// NewTestHarness returns a new TestHarness.
func NewTestHarness() *TestHarness {
	return &TestHarness{
		tlsCertFile:   "/certfile", // will be replaced with real files
		tlsKeyFile:    "/keyfile",  // by WithTestTLSFiles()
		rtaPortNumber: nextTestHarnessPortNum(),
	}
}

func (th *TestHarness) RuntimeArgs() Args {
	var kubeConfigFile string
	if th.fakeKubeConfigFile != nil {
		kubeConfigFile = th.fakeKubeConfigFile.Name()
	}

	return Args{
		CSIAddress:   th.fakeCSIDriverAddr,
		CSITimeout:   20 * time.Millisecond, // UT timeout
		KubeAPIBurst: 99,                    // arbitrary
		KubeAPIQPS:   3.142,                 // arbitrary
		Kubeconfig:   kubeConfigFile,
		GRPCPort:     th.rtaPortNumber,
		TLSCertFile:  th.tlsCertFile,
		TLSKeyFile:   th.tlsKeyFile,
	}
}

func (th *TestHarness) RuntimeForMockCSIDriver(t *testing.T) *Runtime {
	assert.NotNil(t, th.mockCSIDriverConn, "needs WithMockCSIDriver")
	rt := &Runtime{
		Args:    th.RuntimeArgs(),
		CSIConn: th.mockCSIDriverConn,
	}
	return rt
}

func (th *TestHarness) RuntimeForFakeCSIDriver(t *testing.T) *Runtime {
	assert.NotNil(t, th.fakeKubeConfigFile, "needs WithFakeKubeConfig")
	assert.NotEmpty(t, th.fakeCSIDriverAddr, "needs WithFakeCSIDriver")
	rt, err := New(th.RuntimeArgs())
	assert.NoError(t, err)
	return rt
}

// WithFakeKubeConfig creates a kubeconfig file that is needed to communicate
// with the fake CSI driver.
func (th *TestHarness) WithFakeKubeConfig(t *testing.T) *TestHarness {
	// Use the pattern in client-go/tools/client/cmd/client_config_test.go.
	tmpfile, err := os.CreateTemp("", "kubeconfig")
	assert.NoError(t, err, "os.CreateTemp")

	th.fakeKubeConfigFile = tmpfile

	content := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8080
  name: foo-cluster
contexts:
- context:
    cluster: foo-cluster
    user: foo-user
    namespace: bar
  name: foo-context
current-context: foo-context
kind: Config
users:
- name: foo-user
  user:
    token: sha256~iHnHE6AwK3udV9kIJddQpA9W_MxfOZg4BraIO4ywwDY
`
	err = os.WriteFile(tmpfile.Name(), []byte(content), 0666)
	if err != nil {
		th.RemoveFakeKubeConfig(t)
		assert.NoError(t, err, "os.WriteFile")
	}

	return th
}

func (th *TestHarness) RemoveFakeKubeConfig(t *testing.T) {
	utiltesting.CloseAndRemove(t, th.fakeKubeConfigFile)
}

// WithTestTLSFiles will provide temporary but valid TLS files.
func (th *TestHarness) WithTestTLSFiles(t *testing.T) *TestHarness {
	th.tlsGenerator = &testTLSCertGenerator{}
	th.tlsCertFile, th.tlsKeyFile = th.tlsGenerator.GetTLSFiles(t)

	return th
}

func (th *TestHarness) RemoveTestTLSFiles(_ *testing.T) {
	th.tlsGenerator.Cleanup()
}

func (th *TestHarness) WithMockCSIDriver(t *testing.T) *TestHarness {
	// Start the mock server
	mockController := gomock.NewController(t)
	identityServer := driver.NewMockIdentityServer(mockController)
	snapshotMetadataServer := driver.NewMockSnapshotMetadataServer(mockController)
	metricsManager := metrics.NewCSIMetricsManagerForSidecar("" /* driverName */)
	drv := driver.NewMockCSIDriver(&driver.MockCSIDriverServers{
		Identity:         identityServer,
		SnapshotMetadata: snapshotMetadataServer,
	})
	err := drv.Start()
	assert.NoError(t, err, "drv.Start")

	// Create a client connection to it
	addr := drv.Address()
	csiConn, err := connection.Connect(context.Background(), addr, metricsManager)
	if err != nil {
		t.Fatal("Connect", err)
	}

	th.mockCSIDriverConn = csiConn
	th.MockController = mockController
	th.MockCSIDriver = drv
	th.MockCSIIdentityServer = identityServer
	th.MockCSISnapshotMetadataServer = snapshotMetadataServer

	return th
}

func (th *TestHarness) TerminateMockCSIDriver() {
	th.MockController.Finish()
	th.MockCSIDriver.Stop()
	th.mockCSIDriverConn.Close()
}

// WithFakeCSIDriver launches a fake CSIDriver, optionally with a provided SnapshotMetadataServer.
// It initializes the FakeCSIDriverName field.
// If a SnapshotMetadataServer is provided then it initializes the FakeGetPluginCapabilitiesResponse
// field to set the capability for the service.
func (th *TestHarness) WithFakeCSIDriver(t *testing.T, sms csi.SnapshotMetadataServer) *TestHarness {
	// Use the pattern in csi-lib-utils/connection/connection_test.go.
	dir, err := os.MkdirTemp("", "csi")
	assert.NoError(t, err, "os.MkdirTemp")

	th.fakeGRPCServerDir = dir

	th.fakeCSIDriverAddr = path.Join(dir, "server.sock")
	listener, err := net.Listen("unix", th.fakeCSIDriverAddr)
	if err != nil {
		th.TerminateFakeCSIDriver(t)
		assert.NoError(t, err, "net.Listen")
	}

	// the test harness implements the  IdentityServer and the invoker can provide a SnapshotMetadataServer.
	servers := &driver.CSIDriverServers{
		Identity:         th,
		SnapshotMetadata: sms,
	}

	csiDriver := driver.NewCSIDriver(servers)
	err = csiDriver.Start(listener)
	assert.NoError(t, err)

	th.FakeCSIDriver = csiDriver
	th.FakeCSIDriverName = "fake-csi-driver"
	if sms != nil {
		// ensure that the service gets advertised.
		th.FakeGetPluginCapabilitiesResponse = &csi.GetPluginCapabilitiesResponse{
			Capabilities: []*csi.PluginCapability{
				{
					Type: &csi.PluginCapability_Service_{
						Service: &csi.PluginCapability_Service{
							Type: csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE,
						},
					},
				},
			},
		}
	}

	return th
}

func (th *TestHarness) TerminateFakeCSIDriver(t *testing.T) {
	if th.FakeCSIDriver != nil {
		th.FakeCSIDriver.Stop()
	}
	if th.fakeGRPCServerDir != "" {
		os.RemoveAll(th.fakeGRPCServerDir)
	}
}

func (th *TestHarness) AssertErrorStatus(t *testing.T, err error, c codes.Code, msgRegex string) {
	t.Helper()
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, c, st.Code())
	assert.Regexp(t, msgRegex, st.Message())
}

// The test harness fakes an identity server for the fake gRPC service

func (th *TestHarness) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	if th.FakeCSIDriverName != "" {
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: th.FakeCSIDriverName,
		}
		return rspGetPluginInfo, nil
	}

	return nil, status.Error(codes.Unimplemented, "GetPluginInfo is not implemented")
}

func (th *TestHarness) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	if th.FakeProbeResponse != nil {
		return th.FakeProbeResponse, nil
	}
	return nil, status.Error(codes.Unimplemented, "Probe is not implemented")
}

func (th *TestHarness) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	if th.FakeGetPluginCapabilitiesResponse != nil {
		return th.FakeGetPluginCapabilitiesResponse, nil
	}
	return nil, status.Error(codes.Unimplemented, "GetPluginCapabilities is not implemented")
}

// Support to test TLS routines utilizes test cert/key data from crypto/tls/tls_test.go.
type testTLSCertGenerator struct {
	certFile string
	keyFile  string
}

// GetTLSFiles returns the names of temporary cert and key files.
// Use Cleanup() to release these files.
func (tcg *testTLSCertGenerator) GetTLSFiles(t *testing.T) (string, string) {
	tcg.certFile = tcg.writeTempFile(t, "cert", rsaCertPEM)
	tcg.keyFile = tcg.writeTempFile(t, "key", rsaKeyPEM)

	return tcg.certFile, tcg.keyFile
}

func (tcg *testTLSCertGenerator) writeTempFile(t *testing.T, pattern, content string) string {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		tcg.cleanup(nil)
	}
	assert.NoError(t, err)

	if _, err = f.Write([]byte(content)); err != nil {
		tcg.cleanup(f)
	}
	assert.NoError(t, err)

	if err = f.Close(); err != nil {
		tcg.cleanup(f)
	}
	assert.NoError(t, err)

	return f.Name()
}

func (tcg *testTLSCertGenerator) cleanup(f *os.File) {
	if f != nil {
		os.Remove(f.Name())
	}

	tcg.Cleanup()
}

func (tcg *testTLSCertGenerator) Cleanup() {
	if tcg.certFile != "" {
		os.Remove(tcg.certFile)
		tcg.certFile = ""
	}

	if tcg.keyFile != "" {
		os.Remove(tcg.keyFile)
		tcg.keyFile = ""
	}

}

// The following is copied from crypto/tls/tls_test.go
var rsaCertPEM = `-----BEGIN CERTIFICATE-----
MIIB0zCCAX2gAwIBAgIJAI/M7BYjwB+uMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTIwOTEyMjE1MjAyWhcNMTUwOTEyMjE1MjAyWjBF
MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBANLJ
hPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wok/4xIA+ui35/MmNa
rtNuC+BdZ1tMuVCPFZcCAwEAAaNQME4wHQYDVR0OBBYEFJvKs8RfJaXTH08W+SGv
zQyKn0H8MB8GA1UdIwQYMBaAFJvKs8RfJaXTH08W+SGvzQyKn0H8MAwGA1UdEwQF
MAMBAf8wDQYJKoZIhvcNAQEFBQADQQBJlffJHybjDGxRMqaRmDhX0+6v02TUKZsW
r5QuVbpQhH6u+0UgcW0jp9QwpxoPTLTWGXEWBBBurxFwiCBhkQ+V
-----END CERTIFICATE-----
`

var rsaKeyPEM = testingKey(`-----BEGIN RSA TESTING KEY-----
MIIBOwIBAAJBANLJhPHhITqQbPklG3ibCVxwGMRfp/v4XqhfdQHdcVfHap6NQ5Wo
k/4xIA+ui35/MmNartNuC+BdZ1tMuVCPFZcCAwEAAQJAEJ2N+zsR0Xn8/Q6twa4G
6OB1M1WO+k+ztnX/1SvNeWu8D6GImtupLTYgjZcHufykj09jiHmjHx8u8ZZB/o1N
MQIhAPW+eyZo7ay3lMz1V01WVjNKK9QSn1MJlb06h/LuYv9FAiEA25WPedKgVyCW
SmUwbPw8fnTcpqDWE3yTO3vKcebqMSsCIBF3UmVue8YU3jybC3NxuXq3wNm34R8T
xVLHwDXh/6NJAiEAl2oHGGLz64BuAfjKrqwz7qMYr9HCLIe/YsoWq/olzScCIQDi
D2lWusoe2/nEqfDVVWGWlyJ7yOmqaVm/iNUN9B2N2g==
-----END RSA TESTING KEY-----
`)

func testingKey(s string) string { return strings.ReplaceAll(s, "TESTING KEY", "PRIVATE KEY") }

const (
	// dynamic port number range
	minDynamicPortNumber = 49152
	maxDynamicPortNumber = 65535
	// reserve a number of port numbers for back to back test harness invocations.
	thMaxPortsUsableByHarness = 8192
)

var (
	thPortMux     sync.Mutex
	thLastPortNum int
)

func nextTestHarnessPortNum() int {
	thPortMux.Lock()
	defer thPortMux.Unlock()

	if thLastPortNum == 0 {
		// establish a random base with sufficient head room.
		thLastPortNum = rand.IntN(maxDynamicPortNumber-minDynamicPortNumber-thMaxPortsUsableByHarness) + minDynamicPortNumber
	} else {
		thLastPortNum++
	}

	return thLastPortNum
}
