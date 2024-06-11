/*
Copyright 2019 The Kubernetes Authors.

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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"

	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-test/v5/driver"

	"google.golang.org/grpc"
)

func TestCSICheckDriver(t *testing.T) {
	t.Run("empty-driver-name", func(t *testing.T) {
		mr := newMockRuntime(t)
		defer mr.Terminate()

		ctx := context.Background()
		rspGetPluginInfo := &csi.GetPluginInfoResponse{}
		mr.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		rc := mr.runtime()
		err := rc.csiCheckDriver(ctx)
		if err == nil {
			t.Fatal("csiCheckDriver")
		}
	})

	t.Run("get-capabilities-error", func(t *testing.T) {
		mr := newMockRuntime(t)
		defer mr.Terminate()

		ctx := context.Background()
		expDriverName := "csi-driver-name"
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: expDriverName,
		}
		mr.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		mr.mockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error"))

		rc := mr.runtime()
		err := rc.csiCheckDriver(ctx)
		if err == nil {
			t.Fatal("csiCheckDriver")
		}
	})

	t.Run("success", func(t *testing.T) {
		mr := newMockRuntime(t)
		defer mr.Terminate()

		ctx := context.Background()
		expDriverName := "csi-driver-name"
		rspGetPluginInfo := &csi.GetPluginInfoResponse{
			Name: expDriverName,
		}
		mr.mockCSIIdentityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(rspGetPluginInfo, nil)

		rspGetPluginCapabilities := &csi.GetPluginCapabilitiesResponse{}
		mr.mockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(rspGetPluginCapabilities, nil)

		rc := mr.runtime()
		err := rc.csiCheckDriver(ctx)
		if err != nil {
			t.Fatal("csiCheckDriver", err)
		}

		t.Log(rc.driverName)
	})

}

type mockRuntime struct {
	csiConn               *grpc.ClientConn
	mockController        *gomock.Controller
	mockCSIDriver         *driver.MockCSIDriver
	mockCSIIdentityServer *driver.MockIdentityServer
}

func newMockRuntime(t *testing.T) *mockRuntime {
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

	mr := &mockRuntime{
		csiConn:               csiConn,
		mockController:        mockController,
		mockCSIDriver:         drv,
		mockCSIIdentityServer: identityServer,
	}

	return mr
}

func (mr *mockRuntime) Terminate() {
	mr.mockController.Finish()
	mr.mockCSIDriver.Stop()
	mr.csiConn.Close()
}

func (mr *mockRuntime) runtime() *runtimeConfig {
	rc := &runtimeConfig{
		csiConn: mr.csiConn,
	}

	return rc
}
