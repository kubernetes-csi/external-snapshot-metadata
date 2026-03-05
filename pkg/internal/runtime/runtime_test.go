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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNew(t *testing.T) {
	t.Run("validate-args", func(t *testing.T) {
		invalidArgs := []Args{
			{},
			{CSIAddress: "1.2.3.4"},
			{CSIAddress: "1.2.3.4", CSITimeout: time.Hour},
			{CSIAddress: "1.2.3.4", CSITimeout: time.Hour, GRPCPort: 10},
			{CSIAddress: "1.2.3.4", CSITimeout: time.Hour, GRPCPort: 10, TLSCertFile: "/certFile"},
		}
		for i, tc := range invalidArgs {
			t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
				rt, err := New(tc)
				assert.Error(t, err)
				assert.Nil(t, rt)
			})
		}

		t.Run("success", func(t *testing.T) {
			validArgs := Args{
				CSIAddress:  "1.2.3.4",
				CSITimeout:  time.Hour,
				GRPCPort:    10,
				TLSCertFile: "/certFile",
				TLSKeyFile:  "/keyFile",
			}

			err := validArgs.Validate()
			assert.NoError(t, err)
		})
	})

	t.Run("kubeconfig-error", func(t *testing.T) {
		th := NewTestHarness()

		args := th.RuntimeArgs()
		args.CSIAddress = "1.2.3.4"
		args.Kubeconfig = "/invalid/file/path"

		rt, err := New(args)

		assert.Error(t, err, "expected failure in BuildConfigFromFlags")
		assert.Contains(t, err.Error(), "error in kubeconfig")
		assert.Nil(t, rt)
	})

	t.Run("cluster-config-error", func(t *testing.T) {
		th := NewTestHarness()

		t.Setenv("KUBERNETES_SERVICE_HOST", "")

		rt := Runtime{Args: th.RuntimeArgs()}
		err := rt.kubeConnect("", 0, 0)

		assert.Error(t, err, "expected failure in InClusterConfig")
		assert.Contains(t, err.Error(), "error in cluster config")
	})

	t.Run("csi-driver-connection-failure", func(t *testing.T) {
		// indirect invocation of csiConnect via New.
		th := NewTestHarness().WithFakeKubeConfig(t)
		defer th.RemoveFakeKubeConfig(t)

		expArgs := th.RuntimeArgs()
		assert.NotEmpty(t, expArgs.Kubeconfig)

		expArgs.CSIAddress = "0.0.0.0" // force failure

		rt, err := New(expArgs)

		assert.Error(t, err, "expected failure in csiConnect")
		assert.Contains(t, err.Error(), "error connecting to CSI driver")
		assert.Nil(t, rt)
	})

	t.Run("get-driver-name-error", func(t *testing.T) {
		th := NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, nil)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateFakeCSIDriver(t)

		expArgs := th.RuntimeArgs()
		assert.NotEmpty(t, expArgs.Kubeconfig)

		// remove the fake driver name
		th.driverName = ""

		rt, err := New(expArgs)

		assert.Error(t, err, "expected failure in GetDriverName")
		assert.Contains(t, err.Error(), "error getting CSI driver name")
		assert.Nil(t, rt)
	})

	t.Run("success", func(t *testing.T) {
		th := NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, nil)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateFakeCSIDriver(t)

		expDriverName := "csi-driver-name"
		th.driverName = expDriverName

		expArgs := th.RuntimeArgs()
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
		th := NewTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error")).AnyTimes()

		rt := th.RuntimeForMockCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected failure in CSICheckDriver")
		assert.Contains(t, err.Error(), "error waiting for CSI driver to become ready")
	})

	t.Run("get-capabilities-error", func(t *testing.T) {
		th := NewTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		rspProbe := &csi.ProbeResponse{
			Ready: &wrapperspb.BoolValue{Value: true},
		}
		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(rspProbe, nil).AnyTimes()

		th.MockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(nil, errors.New("fake-error"))

		rt := th.RuntimeForMockCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected failure in CSICheckDriver")
		assert.Contains(t, err.Error(), "error getting CSI plugin capabilities")
	})

	t.Run("no-snapshot-metadata-service", func(t *testing.T) {
		th := NewTestHarness().WithMockCSIDriver(t)
		defer th.TerminateMockCSIDriver()

		rspProbe := &csi.ProbeResponse{
			Ready: &wrapperspb.BoolValue{Value: true},
		}
		th.MockCSIIdentityServer.EXPECT().Probe(gomock.Any(), gomock.Any()).Return(rspProbe, nil).AnyTimes()

		rspGetPluginCapabilities := &csi.GetPluginCapabilitiesResponse{}
		th.MockCSIIdentityServer.EXPECT().GetPluginCapabilities(gomock.Any(), gomock.Any()).Return(rspGetPluginCapabilities, nil)

		rt := th.RuntimeForMockCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)
		assert.NotEmpty(t, rt.CSITimeout)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.Error(t, err, "expected capability to not be present")
		assert.Contains(t, err.Error(), "does not support the SNAPSHOT_METADATA_SERVICE")
	})

	t.Run("success", func(t *testing.T) {
		th := NewTestHarness().WithMockCSIDriver(t)
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

		rt := th.RuntimeForMockCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)
		assert.NotEmpty(t, rt.CSITimeout)

		err := rt.WaitTillCSIDriverIsValidated()

		assert.NoError(t, err, "CSICheckDriver")
	})
}
