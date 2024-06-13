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

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/golang/mock/gomock"

	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-test/v5/driver"

	"google.golang.org/grpc"

	utiltesting "k8s.io/client-go/util/testing"
)

func TestClientInit(t *testing.T) {
	t.Run("kubeconfig-error", func(t *testing.T) {
		th := newTestHarness()

		err := th.SidecarService().kubeConnect("/invalid/file/path", 0, 0)
		if err == nil {
			t.Fatal("expected failure in BuildConfigFromFlags", err)
		}
		if !strings.Contains(err.Error(), "error in kubeconfig") {
			t.Fatal("expected a kubeconfig error")
		}
	})

	t.Run("cluster-config-error", func(t *testing.T) {
		th := newTestHarness()

		t.Setenv("KUBERNETES_SERVICE_HOST", "")
		err := th.SidecarService().kubeConnect("", 0, 0)
		if err == nil {
			t.Fatal("expected failure in InClusterConfig")
		}
		if !strings.Contains(err.Error(), "error in cluster config") {
			t.Fatal("expected an in-cluster error")
		}
	})

	t.Run("csi-connection-failure", func(t *testing.T) {
		th := newTestHarness()

		err := th.SidecarService().csiConnect("0.0.0.0")
		if err == nil {
			t.Fatal("expected failure in CSIConnect")
		}
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness().WithFakeKubeConfig(t).WithFakeGRPCServer(t)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateGRPCServer(t)

		expQPS := float32(3.142)
		expBurst := 99

		ss := th.SidecarService()
		err := ss.ClientInit(th.fakeKubeConfigFile.Name(), expQPS, expBurst, th.fakeServerAddr)
		if err != nil {
			t.Fatal("kubeConnect", err)
		}
		if ss.Config == nil {
			t.Fatal("Config not set")
		}
		if ss.KubeClient == nil {
			t.Fatal("KubeClient not set")
		}
		if ss.CSIConn == nil {
			t.Fatal("CSIConn not set")
		}
		if ss.Config.QPS != expQPS {
			t.Fatal("QPS not set")
		}
		if ss.Config.Burst != expBurst {
			t.Fatal("Burst not set")
		}
	})
}

func TestCSICheckDriver(t *testing.T) {
	t.Run("empty-driver-name", func(t *testing.T) {
		th := newTestHarness().WithMockDriver(t)
		defer th.TerminateMockDriver()

		ctx := context.Background()
		rspGetPluginInfo := &csi.GetPluginInfoResponse{}
		th.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		err := th.SidecarService().CSICheckDriver(ctx, th.csiConn)
		if err == nil {
			t.Fatal("csiCheckDriver")
		}
	})

	t.Run("get-capabilities-error", func(t *testing.T) {
		th := newTestHarness().WithMockDriver(t)
		defer th.TerminateMockDriver()

		ctx := context.Background()
		expDriverName := "csi-driver-name"
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: expDriverName,
		}
		th.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		th.mockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error"))

		err := th.SidecarService().CSICheckDriver(ctx, th.csiConn)
		if err == nil {
			t.Fatal("csiCheckDriver")
		}
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness().WithMockDriver(t)
		defer th.TerminateMockDriver()

		ctx := context.Background()
		expDriverName := "csi-driver-name"
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: expDriverName,
		}
		th.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		rspGetPluginCapabilities := &csi.GetPluginCapabilitiesResponse{}
		th.mockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(rspGetPluginCapabilities, nil)

		ss := th.SidecarService()
		err := ss.CSICheckDriver(ctx, th.csiConn)
		if err != nil {
			t.Fatal("csiCheckDriver", err)
		}
		if ss.DriverName != expDriverName {
			t.Fatal("driverName not valid", ss.DriverName)
		}
	})
}

type testHarness struct {
	fakeKubeConfigFile *os.File

	csiConn               *grpc.ClientConn
	mockController        *gomock.Controller
	mockCSIDriver         *driver.MockCSIDriver
	mockCSIIdentityServer *driver.MockIdentityServer

	fakeServerDir  string
	fakeServer     *grpc.Server
	fakeServerWg   sync.WaitGroup
	fakeServerAddr string
}

func newTestHarness() *testHarness {
	return &testHarness{}
}

func (th *testHarness) SidecarService() *SidecarService {
	return &SidecarService{}
}

func (th *testHarness) WithMockDriver(t *testing.T) *testHarness {
	// Start the mock server
	mockController := gomock.NewController(t)
	identityServer := driver.NewMockIdentityServer(mockController)
	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	drv := driver.NewMockCSIDriver(&driver.MockCSIDriverServers{
		Identity: identityServer,
		// TODO: add expected mock services
	})
	drv.Start()

	// Create a client connection to it
	addr := drv.Address()
	csiConn, err := connection.Connect(context.Background(), addr, metricsManager)
	if err != nil {
		t.Fatal("Connect", err)
	}

	th.csiConn = csiConn
	th.mockController = mockController
	th.mockCSIDriver = drv
	th.mockCSIIdentityServer = identityServer

	return th
}

func (th *testHarness) TerminateMockDriver() {
	th.mockController.Finish()
	th.mockCSIDriver.Stop()
	th.csiConn.Close()
}

// WithFakeKubeConfig uses the pattern in client-go/tools/client/cmd/client_config_test.go.
func (th *testHarness) WithFakeKubeConfig(t *testing.T) *testHarness {
	tmpfile, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		t.Fatal("os.CreateTemp", err)
	}

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
		t.Fatal("os.WriteFile", err)
	}

	return th
}

func (th *testHarness) RemoveFakeKubeConfig(t *testing.T) {
	utiltesting.CloseAndRemove(t, th.fakeKubeConfigFile)
}

// WithFakeGRPCServer uses the pattern in csi-lib-utils/connection/connection_test.go.
func (th *testHarness) WithFakeGRPCServer(t *testing.T) *testHarness {
	dir, err := os.MkdirTemp("", "csi")
	if err != nil {
		t.Fatal("os.MkdirTemp", err)
	}

	th.fakeServerDir = dir

	th.fakeServerAddr = path.Join(dir, "server.sock")
	listener, err := net.Listen("unix", th.fakeServerAddr)
	if err != nil {
		th.TerminateGRPCServer(t)
		t.Fatal("net.Listen", err)
	}

	th.fakeServer = grpc.NewServer()

	th.fakeServerWg.Add(1)
	go func() {
		defer th.fakeServerWg.Done()
		if err := th.fakeServer.Serve(listener); err != nil {
			t.Logf("starting server failed: %s", err)
		}
	}()

	return th
}

func (th *testHarness) TerminateGRPCServer(t *testing.T) {
	if th.fakeServer != nil {
		th.fakeServer.Stop()
		th.fakeServerWg.Wait()
	}
	if th.fakeServerDir != "" {
		os.RemoveAll(th.fakeServerDir)
	}
}
