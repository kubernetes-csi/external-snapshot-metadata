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

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func TestGetMetadataDeltaViaGRPCClient(t *testing.T) {
	ctx := context.Background()

	th := newTestHarness().WithMockCSIDriver(t)
	defer th.TerminateMockCSIDriver()
	rtWithCSI := th.RuntimeWithClientAPIsAndMockCSIDriver(t)

	grpcServer := th.StartGRPCServer(t, rtWithCSI)
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
			name:              "csi stream error",
			setCSIDriverReady: true,
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      th.SecurityToken,
				Namespace:          th.Namespace,
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			mockCSIResponse:   true,
			mockCSIError:      fmt.Errorf("stream error"),
			expectStreamError: true,
			expStatusCode:     codes.Internal,
			expStatusMsg:      msgInternalFailedCSIDriverResponse,
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
	th := newTestHarness()
	grpcServer := th.StartGRPCServer(t, th.RuntimeWithClientAPIs())
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
