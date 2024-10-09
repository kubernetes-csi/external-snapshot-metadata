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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/runtime"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/csiclientmocks"
)

func TestGetMetadataAllocatedViaGRPCClient(t *testing.T) {
	ctx := context.Background()
	th := newTestHarness().WithMockCSIDriver(t).WithFakeClientAPIs()
	defer th.TerminateMockCSIDriver()

	grpcServer := th.StartGRPCServer(t, th.Runtime())
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
	th := newTestHarness().WithFakeClientAPIs()
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
	th := newTestHarness().WithFakeClientAPIs()
	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	expSecrets := convStringByteMapToStringStringMap(th.SecretData())

	for _, tc := range []struct {
		name                      string
		apiRequest                *api.GetMetadataAllocatedRequest
		expectedCSIRequest        *csi.GetMetadataAllocatedRequest
		fakeSecret                func(clientgotesting.Action) (bool, runtime.Object, error)
		fakeVolumeSnapshot        func(clientgotesting.Action) (bool, runtime.Object, error)
		fakeVolumeSnapshotContent func(clientgotesting.Action) (bool, runtime.Object, error)
		expectError               bool
		expStatusCode             codes.Code
		expStatusMsgPat           string
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
				SnapshotId:     th.HandleFromSnapshot("snap-1"),
				StartingOffset: 0,
				MaxResults:     256,
				Secrets:        expSecrets,
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
				SnapshotId:     th.HandleFromSnapshot("snap-1"),
				StartingOffset: 35,
				MaxResults:     256,
				Secrets:        expSecrets,
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
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, fmt.Errorf("does not exist")
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotFmt, "test-ns", "snap-doesnt-exist", "does not exist"),
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
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				vs.Status.ReadyToUse = boolPtr(false)
				return true, vs, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableVolumeSnapshotNotReadyFmt, "snap-not-ready"),
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
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				vs.Status.BoundVolumeSnapshotContentName = nil
				return true, vs, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotStatusFmt, "snap-with-no-vsc"),
		},
		{
			// VolumeSnapshotContent resource associated with snapshot doesn't exist
			name: "snapshot-content-get-error",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-content-doesnt-exist",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
			},
			fakeVolumeSnapshot: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
				return true, vs, nil
			},
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, fmt.Errorf("does not exist")
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableFailedToGetVolumeSnapshotContentFmt, th.ContentNameFromSnapshot("snap-content-doesnt-exist"), "does not exist"),
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
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				vsc.Status.ReadyToUse = boolPtr(false)
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableVolumeSnapshotContentNotReadyFmt, th.ContentNameFromSnapshot("snap-with-content-not-ready")),
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
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
				vsc.Status.SnapshotHandle = nil
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.Unavailable,
			expStatusMsgPat: fmt.Sprintf(msgUnavailableInvalidVolumeSnapshotContentStatusFmt, th.ContentNameFromSnapshot("snap-with-content-no-snaphandle")),
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
			fakeVolumeSnapshotContent: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				vsc := th.VolumeSnapshotContent(ga.GetName(), "driver-unexpected")
				return true, vsc, nil
			},
			expectError:     true,
			expStatusCode:   codes.InvalidArgument,
			expStatusMsgPat: fmt.Sprintf(msgInvalidArgumentSnaphotDriverInvalidFmt, "snap-with-invalid-driver", th.DriverName),
		},
		{
			name: "error-fetching-secrets",
			apiRequest: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-with-invalid-secrets",
				Namespace:      "test-ns",
				SecurityToken:  "token",
				StartingOffset: 0,
				MaxResults:     256,
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
			csiReq, err := grpcServer.convertToCSIGetMetadataAllocatedRequest(context.TODO(), tc.apiRequest)
			if tc.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tc.expStatusCode, st.Code())
				assert.Regexp(t, tc.expStatusMsgPat, st.Message())
				return
			}
			assert.NoError(t, err)
			validateCSIGetMetadataAllocatedRequest(t, tc.expectedCSIRequest, csiReq)
		})
	}
}

func validateCSIGetMetadataAllocatedRequest(t *testing.T, csiReq, expectedCSIReq *csi.GetMetadataAllocatedRequest) {
	t.Helper()
	assert.Equal(t, csiReq.SnapshotId, expectedCSIReq.SnapshotId)
	assert.Equal(t, csiReq.StartingOffset, expectedCSIReq.StartingOffset)
	assert.Equal(t, csiReq.MaxResults, expectedCSIReq.MaxResults)
	assert.Equal(t, csiReq.Secrets, expectedCSIReq.Secrets)
}

type mockCSIMetadataAllocatedResponse struct {
	response *csi.GetMetadataAllocatedResponse
	err      error
}

func TestStreamGetMetadataAllocatedResponse(t *testing.T) {
	ctx := context.Background()
	th := newTestHarness().WithMockCSIDriver(t).WithFakeClientAPIs()
	defer th.TerminateMockCSIDriver()

	grpcServer := th.StartGRPCServer(t, th.Runtime())
	defer th.StopGRPCServer(t)

	for _, tc := range []struct {
		name                              string
		req                               *api.GetMetadataAllocatedRequest
		expResponse                       *api.GetMetadataAllocatedResponse
		mockCSIMetadataAllocatedResponses []mockCSIMetadataAllocatedResponse
		mockK8sStreamError                error
		expectStreamError                 bool
		expectK8sStreamError              bool
		expStatusCode                     codes.Code
		expStatusMsg                      string
	}{
		{
			name: "success",
			req: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				StartingOffset: 0,
				MaxResults:     2,
			},
			mockCSIMetadataAllocatedResponses: []mockCSIMetadataAllocatedResponse{
				{
					response: &csi.GetMetadataAllocatedResponse{
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
					response: &csi.GetMetadataAllocatedResponse{
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
			expResponse: &api.GetMetadataAllocatedResponse{
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
			req: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				StartingOffset: 25,
				MaxResults:     1,
			},
			mockCSIMetadataAllocatedResponses: []mockCSIMetadataAllocatedResponse{
				{
					response: &csi.GetMetadataAllocatedResponse{
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

					response: &csi.GetMetadataAllocatedResponse{
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
			expResponse: &api.GetMetadataAllocatedResponse{
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
			req: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				StartingOffset: 25,
				MaxResults:     2,
			},
			mockCSIMetadataAllocatedResponses: []mockCSIMetadataAllocatedResponse{
				{
					response: &csi.GetMetadataAllocatedResponse{
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
			expResponse: &api.GetMetadataAllocatedResponse{
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
			req: &api.GetMetadataAllocatedRequest{
				SnapshotName:   "snap-1",
				StartingOffset: 0,
				MaxResults:     2,
			},
			mockCSIMetadataAllocatedResponses: []mockCSIMetadataAllocatedResponse{
				{
					response: &csi.GetMetadataAllocatedResponse{
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
			mockCSIStream := csiclientmocks.NewMockSnapshotMetadata_GetMetadataAllocatedClient(mockController)

			csiClient.EXPECT().GetMetadataAllocated(gomock.Any(), gomock.Any()).Return(mockCSIStream, nil)
			for _, r := range tc.mockCSIMetadataAllocatedResponses {
				mockCSIStream.EXPECT().Recv().Return(r.response, r.err)
			}
			// mock end of stream
			if !tc.expectStreamError {
				mockCSIStream.EXPECT().Recv().Return(nil, io.EOF)
			}

			csiReq, err := grpcServer.convertToCSIGetMetadataAllocatedRequest(ctx, tc.req)
			assert.NoError(t, err)

			csiStream, err := csiClient.GetMetadataAllocated(ctx, csiReq)
			assert.NoError(t, err)

			sms := &fakeStreamServerSnapshotAllocated{err: tc.mockK8sStreamError}

			errStream := grpcServer.streamGetMetadataAllocatedResponse(sms, csiStream)
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

type fakeStreamServerSnapshotAllocated struct {
	grpc.ServerStream
	response *api.GetMetadataAllocatedResponse
	err      error
}

func (f *fakeStreamServerSnapshotAllocated) Send(m *api.GetMetadataAllocatedResponse) error {
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

func (f *fakeStreamServerSnapshotAllocated) verifyResponse(expectedResponse *api.GetMetadataAllocatedResponse) bool {
	return f.response.String() == expectedResponse.String()
}
