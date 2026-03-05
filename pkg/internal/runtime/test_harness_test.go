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
	"crypto/tls"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRuntimeTestHarness(t *testing.T) {
	t.Run("tls-files", func(t *testing.T) {
		th := NewTestHarness().WithTestTLSFiles(t)
		defer th.RemoveTestTLSFiles(t)

		rta := th.RuntimeArgs()

		cert, err := tls.LoadX509KeyPair(rta.TLSCertFile, rta.TLSKeyFile)
		assert.NoError(t, err)
		assert.NotNil(t, cert)
	})

	t.Run("unique-port-numbers", func(t *testing.T) {
		pNum0 := NewTestHarness().RuntimeArgs().GRPCPort
		assert.GreaterOrEqual(t, pNum0, minDynamicPortNumber)
		assert.LessOrEqual(t, pNum0, maxDynamicPortNumber-thMaxPortsUsableByHarness)
		assert.Equal(t, pNum0, thLastPortNum)

		pNum1 := NewTestHarness().RuntimeArgs().GRPCPort
		assert.True(t, pNum1 == pNum0+1)
		assert.Equal(t, pNum1, thLastPortNum)
	})

	t.Run("fake-identity-server", func(t *testing.T) {
		th := NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, nil)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateFakeCSIDriver(t)

		ctx := context.Background()

		rt := th.RuntimeForFakeCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)

		driverName, err := csirpc.GetDriverName(ctx, rt.CSIConn)
		assert.NoError(t, err)
		assert.NotEmpty(t, driverName)

		th.driverName = ""
		driverName, err = csirpc.GetDriverName(ctx, rt.CSIConn)
		assert.Error(t, err)
		assert.Empty(t, driverName)
		th.AssertErrorStatus(t, err, codes.Unimplemented, "GetPluginInfo is not implemented")

		ready, err := csirpc.Probe(ctx, rt.CSIConn)
		assert.Error(t, err)
		assert.False(t, ready)
		th.AssertErrorStatus(t, err, codes.Unimplemented, "Probe is not implemented")

		th.FakeProbeResponse = &csi.ProbeResponse{Ready: wrapperspb.Bool(true)}
		ready, err = csirpc.Probe(ctx, rt.CSIConn)
		assert.NoError(t, err)
		assert.True(t, ready)

		pc, err := csirpc.GetPluginCapabilities(ctx, rt.CSIConn)
		assert.Error(t, err)
		assert.Nil(t, pc)
		th.AssertErrorStatus(t, err, codes.Unimplemented, "GetPluginCapabilities is not implemented")

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
		pc, err = csirpc.GetPluginCapabilities(ctx, rt.CSIConn)
		assert.NoError(t, err)
		assert.NotNil(t, pc)
		_, found := pc[csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE]
		assert.True(t, found)
	})

	t.Run("fake-snapshot-metadata-server", func(t *testing.T) {
		sms := &testSnapshotMetadataServer{}
		th := NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, sms)
		defer th.RemoveFakeKubeConfig(t)
		defer th.TerminateFakeCSIDriver(t)

		ctx := context.Background()

		rt := th.RuntimeForFakeCSIDriver(t)
		assert.NotNil(t, rt.CSIConn)

		// capability set automatically when the SnapshotMetadataServer is specified.
		pc, err := csirpc.GetPluginCapabilities(ctx, rt.CSIConn)
		assert.NoError(t, err)
		assert.NotNil(t, pc)
		_, found := pc[csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE]
		assert.True(t, found)

		client := csi.NewSnapshotMetadataClient(rt.CSIConn)

		gmaStream, err := client.GetMetadataAllocated(ctx, &csi.GetMetadataAllocatedRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, gmaStream)
		_, err = gmaStream.Recv()
		assert.Error(t, err)
		th.AssertErrorStatus(t, err, codes.Unimplemented, "GetMetadataAllocated is not implemented")

		gmdStream, err := client.GetMetadataDelta(ctx, &csi.GetMetadataDeltaRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, gmdStream)
		_, err = gmdStream.Recv()
		assert.Error(t, err)
		th.AssertErrorStatus(t, err, codes.Unimplemented, "GetMetadataDelta is not implemented")
	})
}

type testSnapshotMetadataServer struct {
	*csi.UnimplementedSnapshotMetadataServer
}

func (s *testSnapshotMetadataServer) GetMetadataAllocated(*csi.GetMetadataAllocatedRequest, csi.SnapshotMetadata_GetMetadataAllocatedServer) error {
	return status.Error(codes.Unimplemented, "GetMetadataAllocated is not implemented")
}

func (s *testSnapshotMetadataServer) GetMetadataDelta(*csi.GetMetadataDeltaRequest, csi.SnapshotMetadata_GetMetadataDeltaServer) error {
	return status.Error(codes.Unimplemented, "GetMetadataDelta is not implemented")
}
