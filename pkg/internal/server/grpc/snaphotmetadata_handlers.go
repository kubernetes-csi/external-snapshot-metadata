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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func (s *Server) GetMetadataAllocated(req *api.GetMetadataAllocatedRequest, stream api.SnapshotMetadata_GetMetadataAllocatedServer) error {
	ctx := stream.Context()

	snapshotHandle, err := s.ValidateGetMetadataAllocatedRequest(ctx, req)
	if err != nil {
		return err
	}

	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	csiClient := csi.NewSnapshotMetadataClient(s.csiConnection())
	csiReq := newCSIGetMetadataAllocatedRequest(ctx, snapshotHandle, req)

	// Invoke CSI Driver's GetMetadataDelta gRPC and stream the response back to client
	csiStream, err := csiClient.GetMetadataAllocated(ctx, csiReq)
	if err != nil {
		return err
	}
	return s.streamGetMetadataAllocatedResponse(stream, csiStream)
}

func (s *Server) GetMetadataDelta(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) error {
	ctx := stream.Context()

	baseSnapshotHandle, targetSnapshotHandle, err := s.ValidateGetMetadataDeltaRequest(ctx, req)
	if err != nil {
		return err
	}

	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	csiClient := csi.NewSnapshotMetadataClient(s.csiConnection())
	csiReq := newCSIGetMetadataDeltaRequest(ctx, baseSnapshotHandle, targetSnapshotHandle, req)

	// Invoke CSI Driver's GetMetadataDelta gRPC and stream the response back to client
	csiStream, err := csiClient.GetMetadataDelta(ctx, csiReq)
	if err != nil {
		return err
	}

	return s.streamGetMetadataDeltaResponse(stream, csiStream)
}

// isCSIDriverReady is a helper for the handlers that returns the appropriate error if the
// CSI driver is not ready.
func (s *Server) isCSIDriverReady() error {
	if s.isReady() {
		return nil
	}

	return status.Errorf(codes.Unavailable, msgUnavailableCSIDriverNotReady)
}
