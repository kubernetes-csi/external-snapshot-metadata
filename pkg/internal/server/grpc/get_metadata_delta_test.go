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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/runtime"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
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
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentSecurityTokenMissing,
		},
		{
			name: "namespace missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentNamespaceMissing,
		},
		{
			name: "base SnapshotName missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				TargetSnapshotName: "snap-2",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentBaseSnapshotNameMissing,
		},
		{
			name: "target SnapshotName missing",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:    "token",
				Namespace:        "test-ns",
				BaseSnapshotName: "snap-1",
			},
			expStatusCode: codes.InvalidArgument,
			expStatusMsg:  msgInvalidArgumentTargetSnapshotNameMissing,
		},
		{
			name: "valid",
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				BaseSnapshotName:   "snap-1",
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
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshot resource with BaseSnapshotName doesn't exist
			name: "base-snapshot-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-doesnt-exist",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshot resource with TargetSnapshotName doesn't exist
			name: "target-snapshot-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshot resource with BaseSnapshotName is not ready
			name: "base-snapshot-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-not-ready",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshot resource with TargetSnapshotName is not ready
			name: "target-snapshot-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// Base snapshot with BoundVolumeSnapshotContent nil
			name: "base-snapshot-content-nil-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-with-no-vsc",
				TargetSnapshotName: "snap-2",
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
			// Target snapshot with BoundVolumeSnapshotContent nil
			name: "target-snapshot-content-nil-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshotContent resource asssociated with base snapshot doesn't exist
			name: "base-snapshot-content-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-content-doesnt-exist",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshotContent resource asssociated with target snapshot doesn't exist
			name: "target-snapshot-content-get-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshotContent associated with base snapshot is not ready
			name: "base-snapshot-content-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-with-content-not-ready",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshotContent associated with target snapshot is not ready
			name: "target-snapshot-content-not-ready-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshotContent associated with base snapshot has empty snapshotHandler
			name: "base-snapshot-content-nil-handler-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-with-content-no-snaphandle",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshotContent associated with target snapshot has empty snapshotHandler
			name: "target-snapshot-content-nil-handler-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
			// VolumeSnapshotContent associated with source snapshot has unexpected driver
			name: "base-snapshot-content-invalid-driver-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-with-invalid-driver",
				TargetSnapshotName: "snap-2",
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
			// VolumeSnapshotContent associated with target snapshot has unexpected driver
			name: "target-snapshot-content-invalid-driver-error",
			apiRequest: &api.GetMetadataDeltaRequest{
				BaseSnapshotName:   "snap-1",
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
				BaseSnapshotName:   "snap-1",
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
