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
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/csiclientmocks"
)

func TestGetMetadataDeltaViaGRPCClient(t *testing.T) {
	ctx := context.Background()
	th := newTestHarness().WithMockCSIDriver(t).WithFakeClientAPIs()
	defer th.TerminateMockCSIDriver()

	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	client := th.GRPCSnapshotMetadataClient(t)

	for _, tc := range []struct {
		name              string
		setCSIDriverReady bool
		req               *api.GetMetadataDeltaRequest
		mockCSIResponse   bool
		mockCSIError      error
		expectStreamError bool
		expStatusCode     codes.Code
		expStatusMsg      string
	}{
		{
			name:              "invalid arguments",
			req:               &api.GetMetadataDeltaRequest{},
			expectStreamError: true,
			expStatusCode:     codes.InvalidArgument,
			expStatusMsg:      msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "invalid token",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken + "FOO",
				Namespace:          th.Namespace,
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expectStreamError: true,
			expStatusCode:     codes.Unauthenticated,
			expStatusMsg:      msgUnauthenticatedUser,
		},
		{
			name: "csi-driver-not-ready",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken,
				Namespace:          th.Namespace,
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expectStreamError: true,
			expStatusCode:     codes.Unavailable,
			expStatusMsg:      msgUnavailableCSIDriverNotReady,
		},
		{
			name:              "csi stream non status error",
			setCSIDriverReady: true,
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken,
				Namespace:          th.Namespace,
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
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
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken,
				Namespace:          th.Namespace,
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
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
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken,
				Namespace:          th.Namespace,
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
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
				th.MockCSISnapshotMetadataServer.EXPECT().GetMetadataDelta(gomock.Any(), gomock.Any()).Return(tc.mockCSIError)
			}

			stream, err := client.GetMetadataDelta(ctx, tc.req)

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

			// Validate metrics are recorded correctly
			metrics, _ := grpcServer.config.Runtime.MetricsManager.GetRegistry().Gather()
			statusFound := 0
			targetSnapshotFound := 0
			baseSnapshotFound := 0

			// Validate that both gauge and controller metrics is recorded
			assert.GreaterOrEqual(t, 2, len(metrics))
			assert.Equal(t, *metrics[0].Name, "process_start_time_seconds")
			assert.Equal(t, *metrics[1].Name, "snapshot_metadata_controller_operations_seconds")

			// Validate grpc_status_code and target_snapshot name
			for _, metric := range metrics[1].Metric {
				for _, labels := range metric.Label {
					expTargetSnapshotName := fmt.Sprintf("%s/%s", tc.req.Namespace, tc.req.TargetSnapshotName)
					expBaseSnapshotId := tc.req.BaseSnapshotId
					if *labels.Name == "grpc_status_code" && *labels.Value == tc.expStatusCode.String() {
						statusFound = 1
					}
					if *labels.Name == "target_snapshot" && *labels.Value == expTargetSnapshotName {
						targetSnapshotFound = 1
					}
					if *labels.Name == "base_snapshot" && *labels.Value == expBaseSnapshotId {
						baseSnapshotFound = 1
					}
				}
			}
			assert.Equal(t, 1, statusFound)
			assert.Equal(t, 1, targetSnapshotFound)
			assert.Equal(t, 1, baseSnapshotFound)
		})
	}
}

func TestValidateGetMetadataDeltaRequest(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()
	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	for _, tc := range []struct {
		name          string
		req           *api.GetMetadataDeltaRequest
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
			req:           &api.GetMetadataDeltaRequest{},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "missing security token",
			req: &api.GetMetadataDeltaRequest{
				Namespace:          "test-ns",
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "namespace missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentNamespaceMissing,
		},
		{
			name: "base SnapshotId missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentBaseSnapshotIdMissing,
		},
		{
			name: "target SnapshotName missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:  "token",
				Namespace:      "test-ns",
				BaseSnapshotId: "snap-1",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentTargetSnapshotNameMissing,
		},
		{
			name: "valid",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
			},
			isValid: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := grpcServer.validateGetMetadataDeltaRequest(tc.req)
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

func TestConvertToCSIGetMetadataDeltaRequest(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()
	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	expSecrets := convStringByteMapToStringStringMap(th.SecretData())

	for _, tc := range []struct {
		name                      string
		apiRequest                *api.GetMetadataDeltaRequest
		expectedCSIRequest        *csi.GetMetadataDeltaRequest
		fakeSecret                func(clientgotesting.Action) (bool, apiruntime.Object, error)
		fakeVolumeSnapshot        func(clientgotesting.Action) (bool, apiruntime.Object, error)
		fakeVolumeSnapshotContent func(clientgotesting.Action) (bool, apiruntime.Object, error)
		expectError               bool
		expStatusCode             codes.Code
		expStatusMsgPat           string
	}{
		{
			// valid case
			name: "success",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-2",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			expectedCSIRequest: &csi.GetMetadataDeltaRequest{
				BaseSnapshotId:   th.HandleFromSnapshot("snap-1"),
				TargetSnapshotId: th.HandleFromSnapshot("snap-2"),
				StartingOffset:   0,
				MaxResults:       256,
				Secrets:          expSecrets,
			},
			expectError: false,
		},
		{
			name: "success",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-2",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     50,
				MaxResults:         256,
			},
			expectedCSIRequest: &csi.GetMetadataDeltaRequest{
				BaseSnapshotId:   th.HandleFromSnapshot("snap-1"),
				TargetSnapshotId: th.HandleFromSnapshot("snap-2"),
				StartingOffset:   50,
				MaxResults:       256,
				Secrets:          expSecrets,
			},
			expectError: false,
		},
		{
			// VolumeSnapshot resource with TargetSnapshotName doesn't exist
			name: "target-snapshot-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-doesnt-exist",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == "snap-doesnt-exist" {
					return true, nil, fmt.Errorf("does not exist")
				}
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				return true, vs, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotFmt, "test-ns", "snap-doesnt-exist", "does not exist"),
		},
		{
			// VolumeSnapshot resource with TargetSnapshotName is not ready
			name: "target-snapshot-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-not-ready",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				if ga.GetName() == "snap-not-ready" {
					vs.Status.ReadyToUse = boolPtr(false)
				}
				return true, vs, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableVolumeSnapshotNotReadyFmt, "snap-not-ready"),
		},
		{
			// Target snapshot with BoundVolumeSnapshotContent nil
			name: "target-snapshot-content-nil-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-with-no-vsc",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				if ga.GetName() == "snap-with-no-vsc" {
					vs.Status.BoundVolumeSnapshotContentName = nil
				}
				return true, vs, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotStatusFmt, "snap-with-no-vsc"),
		},
		{
			// VolumeSnapshotContent resource associated with target snapshot doesn't exist
			name: "target-snapshot-content-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-content-doesnt-exist",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == th.ContentNameFromSnapshot("snap-content-doesnt-exist") {
					return true, nil, fmt.Errorf("does not exist")
				}
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotContentFmt, th.ContentNameFromSnapshot("snap-content-doesnt-exist"), "does not exist"),
		},
		{
			// VolumeSnapshotContent associated with target snapshot is not ready
			name: "target-snapshot-content-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-with-content-not-ready",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				if ga.GetName() == th.ContentNameFromSnapshot("snap-with-content-not-ready") {
					vsc.Status.ReadyToUse = boolPtr(false)
				}
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableVolumeSnapshotContentNotReadyFmt, th.ContentNameFromSnapshot("snap-with-content-not-ready")),
		},
		{
			// VolumeSnapshotContent associated with target snapshot has empty snapshotHandle
			name: "target-snapshot-content-nil-handler-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-with-content-no-snaphandle",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				if ga.GetName() == th.ContentNameFromSnapshot("snap-with-content-no-snaphandle") {
					vsc.Status.SnapshotHandle = nil
				}
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotContentStatusFmt, th.ContentNameFromSnapshot("snap-with-content-no-snaphandle")),
		},
		{
			// VolumeSnapshotContent associated with target snapshot has unexpected driver
			name: "target-snapshot-content-invalid-driver-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-with-invalid-driver",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				if ga.GetName() == th.ContentNameFromSnapshot("snap-with-invalid-driver") {
					vsc.Spec.Driver = *stringPtr("driver-unexpected")
				}
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.InvalidArgument,
			expStatusMsgPat: fmt.Sprintf(msgInvalidArgumentSnaphotDriverInvalidFmt, "snap-with-invalid-driver", th.DriverName),
		},
		{
			name: "error-fetching-secrets",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     th.HandleFromSnapshot("snap-1"),
				TargetSnapshotName: "snap-with-invalid-secrets",
				Namespace:          "test-ns",
				SecurityToken:      "token",
				StartingOffset:     0,
				MaxResults:         256,
			},
			fakeSecret: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, errors.New("secret-get-error")
			},
			expectError:   true,
			expStatusCode: codes.Unavailable,
			// the error comes from an external package so use a regex to match salient pieces.
			expStatusMsgPat: fmt.Sprintf(msgUnavailableFailedToGetCredentials+": .*%s.*$", "secret-get-error"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th.FakeCountReactors()              // must remove test case reactors
			defer th.FakePopPrependedReactors() // to maintain order independence

			if tc.fakeSecret != nil {
				th.FakeKubeClient.PrependReactor("get", "secrets", tc.fakeSecret)
			}
			if tc.fakeVolumeSnapshot != nil {
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", tc.fakeVolumeSnapshot)
			}
			if tc.fakeVolumeSnapshotContent != nil {
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", tc.fakeVolumeSnapshotContent)
			}
			csiReq, err := grpcServer.convertToCSIGetMetadataDeltaRequest(context.TODO(), tc.apiRequest)
			if tc.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expStatusCode, st.Code())
				assert.Regexp(t, tc.expStatusMsgPat, st.Message())
				return
			}
			assert.NoError(t, err)
			validateCSIGetMetadataDeltaRequest(t, tc.expectedCSIRequest, csiReq)
		})
	}
}

func validateCSIGetMetadataDeltaRequest(t *testing.T, csiReq, expectedCSIReq *csi.GetMetadataDeltaRequest) {
	t.Helper()
	assert.Equal(t, csiReq.BaseSnapshotId, expectedCSIReq.BaseSnapshotId)
	assert.Equal(t, csiReq.TargetSnapshotId, expectedCSIReq.TargetSnapshotId)
	assert.Equal(t, csiReq.StartingOffset, expectedCSIReq.StartingOffset)
	assert.Equal(t, csiReq.MaxResults, expectedCSIReq.MaxResults)
	assert.Equal(t, csiReq.Secrets, expectedCSIReq.Secrets)
}

type mockCSIMetadataDeltaResponse struct {
	response *csi.GetMetadataDeltaResponse
	err      error
}

func TestStreamGetMetadataDeltaResponse(t *testing.T) {
	th := newTestHarness().WithMockCSIDriver(t).WithFakeClientAPIs()
	defer th.TerminateMockCSIDriver()

	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	// Test at the trace logging level.
	restoreFn := th.SetKlogVerbosity(HandlerDetailedTraceLogLevel, "Delta")
	defer restoreFn()

	for _, tc := range []struct {
		name                          string
		req                           *api.GetMetadataDeltaRequest
		expResponse                   *api.GetMetadataDeltaResponse
		mockCSIMetadataDeltaResponses []mockCSIMetadataDeltaResponse
		mockK8sStreamError            error
		expectStreamError             bool
		expectK8sStreamError          bool
		expStatusCode                 codes.Code
		expStatusMsg                  string
	}{
		{
			name: "success",
			req: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
				StartingOffset:     0,
				MaxResults:         2,
			},
			mockCSIMetadataDeltaResponses: []mockCSIMetadataDeltaResponse{
				{
					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 0,
								SizeBytes:  1024,
							},
						},
					},
					err: nil,
				},
				{
					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 1,
								SizeBytes:  1024,
							},
							{
								ByteOffset: 2,
								SizeBytes:  1024,
							},
						},
					},
					err: nil,
				},
			},
			expResponse: &api.GetMetadataDeltaResponse{
				BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
				VolumeCapacityBytes: 1024 * 1024 * 1024,
				BlockMetadata: []*api.BlockMetadata{
					{
						ByteOffset: 0,
						SizeBytes:  1024,
					},
					{
						ByteOffset: 1,
						SizeBytes:  1024,
					},
					{
						ByteOffset: 2,
						SizeBytes:  1024,
					},
				},
			},
		},
		// Starting offset non-zero
		{
			name: "success",
			req: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
				StartingOffset:     25,
				MaxResults:         1,
			},
			mockCSIMetadataDeltaResponses: []mockCSIMetadataDeltaResponse{
				{
					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 25,
								SizeBytes:  1024,
							},
						},
					},
					err: nil,
				},
				{

					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 26,
								SizeBytes:  1024,
							},
						},
					},
					err: nil,
				},
			},
			expResponse: &api.GetMetadataDeltaResponse{
				BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
				VolumeCapacityBytes: 1024 * 1024 * 1024,
				BlockMetadata: []*api.BlockMetadata{
					{
						ByteOffset: 25,
						SizeBytes:  1024,
					},
					{
						ByteOffset: 26,
						SizeBytes:  1024,
					},
				},
			},
		},

		// CSI driver response error
		{
			name: "csi-stream-error",
			req: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
				StartingOffset:     25,
				MaxResults:         2,
			},
			mockCSIMetadataDeltaResponses: []mockCSIMetadataDeltaResponse{
				{
					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 25,
								SizeBytes:  1024,
							},
						},
					},
					err: nil,
				},
				{
					response: nil,
					err:      io.ErrUnexpectedEOF,
				},
			},
			// expect incomplete response
			expResponse: &api.GetMetadataDeltaResponse{
				BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
				VolumeCapacityBytes: 1024 * 1024 * 1024,
				BlockMetadata: []*api.BlockMetadata{
					{
						ByteOffset: 25,
						SizeBytes:  1024,
					},
				},
			},
			expectStreamError: true,
			expStatusCode:     codes.Internal,
			expStatusMsg:      msgInternalFailedCSIDriverResponse,
		},

		// K8s stream response error
		{
			name: "k8s-stream-error",
			req: &api.GetMetadataDeltaRequest{
				BaseSnapshotId:     "snap-1",
				TargetSnapshotName: "snap-2",
				StartingOffset:     0,
				MaxResults:         2,
			},
			mockCSIMetadataDeltaResponses: []mockCSIMetadataDeltaResponse{
				{
					response: &csi.GetMetadataDeltaResponse{
						BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
						VolumeCapacityBytes: 1024 * 1024 * 1024,
						BlockMetadata: []*csi.BlockMetadata{
							{
								ByteOffset: 0,
								SizeBytes:  1024,
							},
						},
					},
				},
			},
			mockK8sStreamError: errors.New("K8s stream send error"),
			expectStreamError:  true,
			expStatusCode:      codes.Internal,
			expStatusMsg:       msgInternalFailedToSendResponse,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// mock CSI server streaming
			mockController := gomock.NewController(t)
			defer mockController.Finish()
			csiClient := csiclientmocks.NewMockSnapshotMetadataClient(mockController)
			mockCSIStream := csiclientmocks.NewMockSnapshotMetadata_GetMetadataDeltaClient(mockController)

			csiClient.EXPECT().GetMetadataDelta(gomock.Any(), gomock.Any()).Return(mockCSIStream, nil)
			for _, r := range tc.mockCSIMetadataDeltaResponses {
				mockCSIStream.EXPECT().Recv().Return(r.response, r.err)
			}
			// mock end of stream
			if !tc.expectStreamError {
				mockCSIStream.EXPECT().Recv().Return(nil, io.EOF)
			}

			sms := &fakeStreamServerSnapshotDelta{err: tc.mockK8sStreamError}
			ctx := grpcServer.getMetadataDeltaContextWithLogger(tc.req, sms)

			csiReq, err := grpcServer.convertToCSIGetMetadataDeltaRequest(ctx, tc.req)
			assert.NoError(t, err)

			csiStream, err := csiClient.GetMetadataDelta(ctx, csiReq)
			assert.NoError(t, err)

			errStream := grpcServer.streamGetMetadataDeltaResponse(ctx, sms, csiStream)
			if tc.expectStreamError {
				assert.NoError(t, err)
				st, ok := status.FromError(errStream)
				assert.True(t, ok)
				assert.Equal(t, tc.expStatusCode, st.Code())
				assert.ErrorContains(t, errStream, tc.expStatusMsg)
				// Expect incomplete/partial response
				assert.Equal(t, true, sms.verifyResponse(tc.expResponse))
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, true, sms.verifyResponse(tc.expResponse))
		})
	}
}

type fakeStreamServerSnapshotDelta struct {
	grpc.ServerStream
	response *api.GetMetadataDeltaResponse
	err      error
}

func (f *fakeStreamServerSnapshotDelta) Context() context.Context {
	return context.Background()
}

func (f *fakeStreamServerSnapshotDelta) Send(m *api.GetMetadataDeltaResponse) error {
	if f.err != nil {
		return f.err
	}
	if f.response == nil {
		f.response = m
		return nil
	}
	f.response.BlockMetadata = append(f.response.BlockMetadata, m.BlockMetadata...)
	return nil
}

func (f *fakeStreamServerSnapshotDelta) verifyResponse(expectedResponse *api.GetMetadataDeltaResponse) bool {
	return f.response.String() == expectedResponse.String()
}

func TestGetMetadataDeltaClientErrorHandling(t *testing.T) {
	t.Run("client-cancels-context", func(t *testing.T) {
		sms, th := newSMSHarnessForCtxPropagation(t, 0)
		defer sms.cleanup(t)

		// create the cancelable application client context
		ctx, cancelFn := context.WithCancel(context.Background())

		// make the RPC call
		client := th.GRPCSnapshotMetadataClient(t)
		clientStream, err := client.GetMetadataDelta(ctx, &api.GetMetadataDeltaRequest{
			SecurityToken:      th.SecurityToken,
			Namespace:          th.Namespace,
			BaseSnapshotId:     "snap-1",
			TargetSnapshotName: "snap-2",
		})
		assert.NoError(t, err)
		assert.NotNil(t, clientStream)

		sms.synchronizeBeforeCancel()

		r1, e1 := clientStream.Recv() // get the first response
		assert.NoError(t, e1)
		assert.NotNil(t, r1)

		// the client cancels the context
		cancelFn()

		r2, e2 := clientStream.Recv() // fail because ctx is canceled
		assert.Error(t, e2)
		assert.ErrorContains(t, e2, context.Canceled.Error())
		assert.Nil(t, r2)

		sms.synchronizeAfterCancel()

		// Check the fake driver handler status
		sms.mux.Lock()
		defer sms.mux.Unlock()
		assert.True(t, sms.handlerCalled)
		assert.NoError(t, sms.send1Err)
		assert.ErrorIs(t, sms.streamCtxErr, context.Canceled)
		assert.Error(t, sms.send2Err)
		assert.ErrorContains(t, sms.send2Err, context.Canceled.Error())
	})

	t.Run("sidecar-deadline-exceeded", func(t *testing.T) {
		// arrange for the sidecar to timeout quickly
		sms, th := newSMSHarnessForCtxPropagation(t, time.Millisecond*10)
		defer sms.cleanup(t)

		// make the RPC call
		client := th.GRPCSnapshotMetadataClient(t)
		clientStream, err := client.GetMetadataDelta(context.Background(), &api.GetMetadataDeltaRequest{
			SecurityToken:      th.SecurityToken,
			Namespace:          th.Namespace,
			BaseSnapshotId:     "snap-1",
			TargetSnapshotName: "snap-2",
		})
		assert.NoError(t, err)
		assert.NotNil(t, clientStream)

		sms.synchronizeBeforeCancel()

		// do not attempt to receive anything

		sms.synchronizeAfterCancel()

		// Check the fake driver handler status
		sms.mux.Lock()
		defer sms.mux.Unlock()
		assert.True(t, sms.handlerCalled)
		assert.NoError(t, sms.send1Err)
		assert.Error(t, sms.send2Err)
		// its a bit uncertain as to which context error we get in the handler
		re := context.DeadlineExceeded.Error() + "|" + context.Canceled.Error()
		assert.Regexp(t, re, sms.send2Err.Error())
	})
}
