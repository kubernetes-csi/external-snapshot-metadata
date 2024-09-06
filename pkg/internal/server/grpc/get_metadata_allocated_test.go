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

package grpc

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/runtime"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func TestGetMetadataAllocatedViaGRPCClient(t *testing.T) {
	ctx := context.Background()

	th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
	defer th.TerminateMockCSIDriver()
	rtWithCSI := th.RuntimeWithMockCSIDriver(t)

	grpcServer := th.StartGRPCServer(t, rtWithCSI)
	defer th.StopGRPCServer(t)

	client := th.GRPCSnapshotMetadataClient(t)

	for _, tc := range []struct {
		name              string
		setCSIDriverReady bool
		req               *api.GetMetadataAllocatedRequest
		mockCSIResponse   bool
		mockCSIError      error
		expectStreamError bool
		expStatusCode     codes.Code
		expStatusMsg      string
	}{
		{
			name:              "invalid arguments",
			req:               &api.GetMetadataAllocatedRequest{},
			expectStreamError: true,
			expStatusCode:     codes.InvalidArgument,
			expStatusMsg:      msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "invalid token",
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: th.SecurityToken + "FOO",
				Namespace:     th.Namespace,
				SnapshotName:  "snap-1",
			},
			expectStreamError: true,
			expStatusCode:     codes.Unauthenticated,
			expStatusMsg:      msgUnauthenticatedUser,
		},
		{
			name: "csi-driver-not-ready",
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: th.SecurityToken,
				Namespace:     th.Namespace,
				SnapshotName:  "snap-1",
			},
			expectStreamError: true,
			expStatusCode:     codes.Unavailable,
			expStatusMsg:      msgUnavailableCSIDriverNotReady,
		},
		{
			name:              "csi stream non status error",
			setCSIDriverReady: true,
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: th.SecurityToken,
				Namespace:     th.Namespace,
				SnapshotName:  "snap-1",
			},
			mockCSIResponse:   true,
			mockCSIError:      fmt.Errorf("not a status error"),
			expectStreamError: true,
			expStatusCode:     codes.Internal,
			expStatusMsg:      msgInternalFailedCSIDriverResponse,
		},
		{
			name:              "csi stream status error",
			setCSIDriverReady: true,
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: th.SecurityToken,
				Namespace:     th.Namespace,
				SnapshotName:  "snap-1",
			},
			mockCSIResponse:   true,
			mockCSIError:      status.Errorf(codes.Aborted, "is a status error"),
			expectStreamError: true,
			expStatusCode:     codes.Aborted, // code unchanged
			expStatusMsg:      "is a status error",
		},
		{
			name:              "success",
			setCSIDriverReady: true,
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: th.SecurityToken,
				Namespace:     th.Namespace,
				SnapshotName:  "snap-1",
			},
			mockCSIResponse:   true,
			mockCSIError:      nil,
			expectStreamError: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setCSIDriverReady {
				grpcServer.CSIDriverIsReady()
			}

			if tc.mockCSIResponse {
				th.MockCSISnapshotMetadataServer.EXPECT().GetMetadataAllocated(gomock.Any(), gomock.Any()).Return(tc.mockCSIError)
			}

			stream, err := client.GetMetadataAllocated(ctx, tc.req)

			assert.NoError(t, err)

			_, errStream := stream.Recv()

			if tc.expectStreamError {
				assert.NotNil(t, errStream)
				st, ok := status.FromError(errStream)
				assert.True(t, ok)
				assert.Equal(t, tc.expStatusCode, st.Code())
				assert.ErrorContains(t, errStream, tc.expStatusMsg)
			} else if errStream != nil {
				assert.ErrorIs(t, errStream, io.EOF)
			}
		})
	}
}

func TestValidateGetMetadataAllocatedRequest(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	for _, tc := range []struct {
		name          string
		req           *api.GetMetadataAllocatedRequest
		isValid       bool
		expStatusCode codes.Code
		expStatusMsg  string
	}{
		{
			name:          "nil",
			req:           nil,
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name:          "empty arguments",
			req:           &api.GetMetadataAllocatedRequest{},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "missing security token",
			req: &api.GetMetadataAllocatedRequest{
				Namespace:    "test-ns",
				SnapshotName: "snap-1",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "namespace missing",
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				SnapshotName:  "snap-1",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentNamespaceMissing,
		},
		{
			name: "snapshotName missing",
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				Namespace:     "test-ns",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSnaphotNameMissing,
		},
		{
			name: "valid",
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				Namespace:     "test-ns",
				SnapshotName:  "snap-1",
			},
			isValid: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := grpcServer.validateGetMetadataAllocatedRequest(tc.req)
			if !tc.isValid {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expStatusCode, st.Code())
				assert.Equal(t, tc.expStatusMsg, st.Message())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConvertToCSIGetMetadataAllocatedRequest(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
	rt := th.Runtime()
	grpcServer := th.StartGRPCServer(t, rt)
	defer th.StopGRPCServer(t)

	for _, tc := range []struct {
		name                      string
		apiRequest                *api.GetMetadataAllocatedRequest
		expectedCSIRequest        *csi.GetMetadataAllocatedRequest
		mockVolumeSnapshot        func(clientgotesting.Action) (bool, runtime.Object, error)
		mockVolumeSnapshotContent func(clientgotesting.Action) (bool, runtime.Object, error)
		expectError               bool
		expStatusCode             codes.Code
		expStatusMsg              string
	}{
		{
			// valid case
			name: "success",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			expectedCSIRequest: &csi.GetMetadataAllocatedRequest{
				SnapshotId:     "snap-1-id",
				StartingOffset: 0,
				MaxResults:     256,
			},
			expectError: false,
		},
		{
			name: "success",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 35,
				MaxResults:     256,
			},
			expectedCSIRequest: &csi.GetMetadataAllocatedRequest{
				SnapshotId:     "snap-1-id",
				StartingOffset: 35,
				MaxResults:     256,
			},
			expectError: false,
		},
		{
			// VolumeSnapshot resource with SnapshotName doesn't exist
			name: "snapshot-get-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-doesnt-exist",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, fmt.Errorf("does not exist")
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotFmt, "does not exist"),
		},
		{
			// VolumeSnapshot resource with SnapshotName is not ready
			name: "snapshot-not-ready-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-not-ready",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := volumeSnapshot(ga.GetName(), ga.GetNamespace())
				vs.Status.ReadyToUse = boolPtr(false)
				return true, vs, nil
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableVolumeSnapshotNotReadyFmt, "snap-not-ready"),
		},
		{
			// Snapshot with BoundVolumeSnapshotContent nil
			name: "snapshot-content-nil-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-with-no-vsc",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := volumeSnapshot(ga.GetName(), ga.GetNamespace())
				vs.Status.BoundVolumeSnapshotContentName = nil
				return true, vs, nil
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotStatusFmt, "snap-with-no-vsc"),
		},

		{
			// VolumeSnapshotContent resource asssociated with snapshot doesn't exist
			name: "snapshot-content-get-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-content-doesnt-exist",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := volumeSnapshot(ga.GetName(), ga.GetNamespace())
				return true, vs, nil
			},
			mockVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, fmt.Errorf("does not exist")
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotContentFmt, "does not exist"),
		},
		{
			// VolumeSnapshotContent associated with snapshot is not ready
			name: "snapshot-content-not-ready-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-with-content-not-ready",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := volumeSnapshotContent(ga.GetName(), th.DriverName)
				vsc.Status.ReadyToUse = boolPtr(false)
				return true, vsc, nil
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableVolumeSnapshotContentNotReadyFmt, "snap-with-content-not-ready"),
		},
		{
			// VolumeSnapshotContent associated with snapshot has empty snapshotHandler
			name: "snapshot-content-nil-handler-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-with-content-no-snaphandle",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := volumeSnapshotContent(ga.GetName(), th.DriverName)
				vsc.Status.SnapshotHandle = nil
				return true, vsc, nil
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			expStatusMsg:  fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotContentStatusFmt, "snap-with-content-no-snaphandle"),
		},
		{
			// VolumeSnapshotContent associated with snapshot has unexpected driver
			name: "snapshot-content-invalid-driver-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-with-invalid-driver",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			mockVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := volumeSnapshotContent(ga.GetName(), "driver-unexpected")
				return true, vsc, nil
			},
			expectError:   true,
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  fmt.Sprintf(msgInvalidArgumentSnaphotDriverInvalidFmt, "snap-with-invalid-driver", th.DriverName),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockVolumeSnapshot != nil {
				th.fakeSnapshotClient.PrependReactor("get", "volumesnapshots", tc.mockVolumeSnapshot)
			}
			if tc.mockVolumeSnapshotContent != nil {
				th.fakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", tc.mockVolumeSnapshotContent)
			}
			csiReq, err := grpcServer.convertToCSIGetMetadataAllocatedRequest(context.TODO(), tc.apiRequest)
			if tc.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, st.Code(), tc.expStatusCode)
				assert.Equal(t, st.Message(), tc.expStatusMsg)
				return
			}
			assert.NoError(t, err)
			validateCSIGetMetadataAllocatedRequest(t, tc.expectedCSIRequest, csiReq)
		})
	}
}

func validateCSIGetMetadataAllocatedRequest(t *testing.T, csiReq, expectedCSIReq *csi.GetMetadataAllocatedRequest) {
	assert.Equal(t, csiReq.SnapshotId, expectedCSIReq.SnapshotId)
	assert.Equal(t, csiReq.StartingOffset, expectedCSIReq.StartingOffset)
	assert.Equal(t, csiReq.MaxResults, expectedCSIReq.MaxResults)
}
