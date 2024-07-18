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

import (
	"context"
	"errors"
	"net"
	"os"
	"path"
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
	"google.golang.org/protobuf/types/known/wrapperspb"

	utiltesting "k8s.io/client-go/util/testing"
)

func TestNew(t *testing.T) {
	t.Run("invalid-args", func(t *testing.T) {
		args := Args{}

		rt, err := New(args)
		assert.Error(t, err)
		assert.Nil(t, rt)
		assert.Contains(t, err.Error(), "CSIAddress is required")

		args.CSIAddress = "1.2.3.4"
		rt, err = New(args)
		assert.Error(t, err)
		assert.Nil(t, rt)
		assert.Contains(t, err.Error(), "CSITimeout is required")
	})

	t.Run("kubeconfig-error", func(t *testing.T) {
		th := newTestHarness()

		args := th.Runtime().Args // use harness runtime for the args
		args.CSIAddress = "1.2.3.4"
		args.Kubeconfig = "/invalid/file/path"

		rt, err := New(args)

		assert.Error(t, err, "expected failure in BuildConfigFromFlags")
		assert.Contains(t, err.Error(), "error in kubeconfig")
		assert.Nil(t, rt)
	})

	t.Run("cluster-config-error", func(t *testing.T) {
		th := newTestHarness()

		t.Setenv("KUBERNETES_SERVICE_HOST", "")

		err := th.Runtime().kubeConnect("", 0, 0)

		assert.Error(t, err, "expected failure in InClusterConfig")
		assert.Contains(t, err.Error(), "error in cluster config")
	})

	t.Run("csi-driver-connection-failure", func(t *testing.T) {
		// indirect invocation of csiConnect via New.
		th := newTestHarness().WithFakeKubeConfig(t)
		defer th.RemoveFakeKubeConfig(t)

		expArgs := th.Runtime().Args // use harness runtime for the args
		assert.NotEmpty(t, expArgs.Kubeconfig)

		expArgs.CSIAddress = "0.0.0.0" // force failure

		rt, err := New(expArgs)

		assert.Error(t, err, "expected failure in csiConnect")
		assert.Contains(t, err.Error(), "error connecting to CSI driver")
		assert.Nil(t, rt)
	})

	t.Run("get-driver-name-error", func(t *testing.T) {
		th := newTestHarness().WithFakeKubeConfig(t).WithFakeGRPCServer(t)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateGRPCServer(t)

		expArgs := th.Runtime().Args // use the harness args
		assert.NotEmpty(t, expArgs.Kubeconfig)

		rt, err := New(expArgs)

		assert.Error(t, err, "expected failure in GetDriverName")
		assert.Contains(t, err.Error(), "error getting CSI driver name")
		assert.Nil(t, rt)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness().WithFakeKubeConfig(t).WithFakeGRPCServer(t)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateGRPCServer(t)

		expDriverName := "csi-driver-name"
		th.FakeGRPCDriverName = expDriverName

		expArgs := th.Runtime().Args // use the harness args
		assert.NotEmpty(t, expArgs.Kubeconfig)

		rt, err := New(expArgs)

		assert.NoError(t, err)
		assert.NotNil(t, rt)
		assert.NotNil(t, rt.KubeClient)
		assert.NotNil(t, rt.CBTClient)
		assert.NotNil(t, rt.Config)
		assert.Equal(t, expArgs.KubeAPIQPS, rt.Config.QPS)
		assert.Equal(t, expArgs.KubeAPIBurst, rt.Config.Burst)
		assert.Equal(t, expArgs.Kubeconfig, rt.Kubeconfig)
		assert.NotNil(t, rt.CSIConn)
		assert.NotNil(t, rt.MetricsManager)
		assert.Equal(t, expDriverName, rt.DriverName)
	})
}

func TestWaitTillCSIDriverIsValidated(t *testing.T) {
	t.Run("probe-error", func(t *testing.T) {
		th := newTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error")).AnyTimes()

		rt := th.Runtime()
		assert.NotNil(t, rt.CSIConn)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected failure in CSICheckDriver")
		assert.Contains(t, err.Error(), "error waiting for CSI driver to become ready")
	})

	t.Run("get-capabilities-error", func(t *testing.T) {
		th := newTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		rspProbe := &csi.ProbeResponse{
			Ready: &wrapperspb.BoolValue{Value: true},
		}
		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(rspProbe, nil).AnyTimes()

		th.MockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error"))

		rt := th.Runtime()
		assert.NotNil(t, rt.CSIConn)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected failure in CSICheckDriver")
		assert.Contains(t, err.Error(), "error getting CSI plugin capabilities")
	})

	t.Run("no-snapshot-metadata-service", func(t *testing.T) {
		th := newTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		rspProbe := &csi.ProbeResponse{
			Ready: &wrapperspb.BoolValue{Value: true},
		}
		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(rspProbe, nil).AnyTimes()

		rspGetPluginCapabilities := &csi.GetPluginCapabilitiesResponse{}
		th.MockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(rspGetPluginCapabilities, nil)

		rt := th.Runtime()
		assert.NotNil(t, rt.CSIConn)
		assert.NotEmpty(t, rt.CSITimeout)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected capability to not be present")
		assert.Contains(t, err.Error(), "does not support the SNAPSHOT_METADATA_SERVICE")
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		rspProbe := &csi.ProbeResponse{
			Ready: &wrapperspb.BoolValue{Value: true},
		}
		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(rspProbe, nil).AnyTimes()

		rspGetPluginCapabilities := &csi.GetPluginCapabilitiesResponse{
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
		th.MockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(rspGetPluginCapabilities, nil)

		rt := th.Runtime()
		assert.NotNil(t, rt.CSIConn)
		assert.NotEmpty(t, rt.CSITimeout)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.NoError(t, err, "CSICheckDriver")
	})
}

type testHarness struct {
	CSIDriverConn         *grpc.ClientConn
	MockController        *gomock.Controller
	MockCSIDriver         *driver.MockCSIDriver
	MockCSIIdentityServer *driver.MockIdentityServer

	fakeGRPCServerDir  string
	fakeGRPCServer     *grpc.Server
	fakeServerWg       sync.WaitGroup
	FakeGRPCServerAddr string
	FakeGRPCDriverName string // fake CSI driver name

	FakeKubeConfigFile *os.File

	// for the mock IdentityServer
	*csi.UnimplementedIdentityServer
}

func newTestHarness() *testHarness {
	return &testHarness{}
}

func (th *testHarness) Runtime() *Runtime {
	rt := &Runtime{
		Args: Args{
			CSIAddress:   th.FakeGRPCServerAddr, // needs WithFakeGRPCServer
			CSITimeout:   20 * time.Millisecond, // UT timeout
			KubeAPIBurst: 99,                    // arbitrary
			KubeAPIQPS:   3.142,                 // arbitrary
		},
		CSIConn: th.CSIDriverConn, // needs WithMockCSIDriver
	}
	if th.FakeKubeConfigFile != nil { // needs WithFakeKubeConfig
		rt.Kubeconfig = th.FakeKubeConfigFile.Name()
	}
	return rt
}

// WithFakeKubeConfig uses the pattern in client-go/tools/client/cmd/client_config_test.go.
func (th *testHarness) WithFakeKubeConfig(t *testing.T) *testHarness {
	tmpfile, err := os.CreateTemp("", "kubeconfig")
	assert.NoError(t, err, "os.CreateTemp")

	th.FakeKubeConfigFile = tmpfile

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

func (th *testHarness) RemoveFakeKubeConfig(t *testing.T) {
	utiltesting.CloseAndRemove(t, th.FakeKubeConfigFile)
}

func (th *testHarness) WithMockCSIDriver(t *testing.T) *testHarness {
	// Start the mock server
	mockController := gomock.NewController(t)
	identityServer := driver.NewMockIdentityServer(mockController)
	metricsManager := metrics.NewCSIMetricsManagerForSidecar("" /* driverName */)
	drv := driver.NewMockCSIDriver(&driver.MockCSIDriverServers{
		Identity: identityServer,
		// TODO: add expected mock services
	})
	err := drv.Start()
	assert.NoError(t, err, "drv.Start")

	// Create a client connection to it
	addr := drv.Address()
	csiConn, err := connection.Connect(context.Background(), addr, metricsManager)
	if err != nil {
		t.Fatal("Connect", err)
	}

	th.CSIDriverConn = csiConn
	th.MockController = mockController
	th.MockCSIDriver = drv
	th.MockCSIIdentityServer = identityServer

	return th
}

func (th *testHarness) TerminateMockCSIDriver() {
	th.MockController.Finish()
	th.MockCSIDriver.Stop()
	th.CSIDriverConn.Close()
}

// WithFakeGRPCServer uses the pattern in csi-lib-utils/connection/connection_test.go.
func (th *testHarness) WithFakeGRPCServer(t *testing.T, opt ...grpc.ServerOption) *testHarness {
	dir, err := os.MkdirTemp("", "csi")
	assert.NoError(t, err, "os.MkdirTemp")

	th.fakeGRPCServerDir = dir

	th.FakeGRPCServerAddr = path.Join(dir, "server.sock")
	listener, err := net.Listen("unix", th.FakeGRPCServerAddr)
	if err != nil {
		th.TerminateGRPCServer(t)
		assert.NoError(t, err, "net.Listen")
	}

	th.fakeGRPCServer = grpc.NewServer()
	csi.RegisterIdentityServer(th.fakeGRPCServer, th) // the harness implements an identity server

	th.fakeServerWg.Add(1)
	go func() {
		defer th.fakeServerWg.Done()
		if err := th.fakeGRPCServer.Serve(listener); err != nil {
			assert.NoError(t, err, "fakeServer.Serve")
		}
	}()

	return th
}

func (th *testHarness) TerminateGRPCServer(t *testing.T) {
	if th.fakeGRPCServer != nil {
		th.fakeGRPCServer.Stop()
		th.fakeServerWg.Wait()
	}
	if th.fakeGRPCServerDir != "" {
		os.RemoveAll(th.fakeGRPCServerDir)
	}
}

// The test harness fakes an identity server for the fake gRPC service

func (th *testHarness) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	if th.FakeGRPCDriverName != "" {
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: th.FakeGRPCDriverName,
		}
		return rspGetPluginInfo, nil
	}

	return nil, status.Error(codes.Unimplemented, "Unimplemented")
}

func (th *testHarness) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unimplemented")
}

func (th *testHarness) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unimplemented")
}
