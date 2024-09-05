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
	"io"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func (s *Server) GetMetadataAllocated(req *api.GetMetadataAllocatedRequest, stream api.SnapshotMetadata_GetMetadataAllocatedServer) error {
	ctx := stream.Context()

	if err := s.validateGetMetadataAllocatedRequest(req); err != nil {
		return err
	}

	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	csiReq, err := s.convertToCSIGetMetadataAllocatedRequest(ctx, req)
	if err != nil {
		return err
	}

	// Invoke the CSI Driver's GetMetadataDelta gRPC and stream the response back to client
	csiStream, err := csi.NewSnapshotMetadataClient(s.csiConnection()).GetMetadataAllocated(ctx, csiReq)
	if err != nil {
		return err
	}

	return s.streamGetMetadataAllocatedResponse(stream, csiStream)
}

func (s *Server) validateGetMetadataAllocatedRequest(req *api.GetMetadataAllocatedRequest) error {
	if len(req.GetSecurityToken()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentSecurityTokenMissing)
	}

	if len(req.GetNamespace()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentNamespaceMissing)
	}

	if len(req.GetSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotNameMissing)
	}

	return nil
}

func (s *Server) convertToCSIGetMetadataAllocatedRequest(ctx context.Context, req *api.GetMetadataAllocatedRequest) (*csi.GetMetadataAllocatedRequest, error) {
	snapshotHandle, driver, err := s.getVolSnapshotInfo(ctx, req.Namespace, req.SnapshotName)
	if err != nil {
		return nil, err
	}

	if driver != s.driverName() {
		return nil, status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, req.SnapshotName, s.driverName())
	}

	return &csi.GetMetadataAllocatedRequest{
		SnapshotId:     snapshotHandle,
		StartingOffset: req.StartingOffset,
		MaxResults:     req.MaxResults,
		//TODO: set Secrets field
	}, nil
}

func (s *Server) streamGetMetadataAllocatedResponse(clientStream api.SnapshotMetadata_GetMetadataAllocatedServer, csiStream csi.SnapshotMetadata_GetMetadataAllocatedClient) error {
	for {
		csiResp, err := csiStream.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return status.Errorf(codes.Internal, msgInternalFailedCSIDriverResponseFmt, err)
		}

		clientResp := s.convertToGetMetadataAllocatedResponse(csiResp)
		if err := clientStream.Send(clientResp); err != nil {
			return status.Errorf(codes.Internal, msgInternalFailedtoSendResponseFmt, err)
		}
	}
}

func (s *Server) convertToGetMetadataAllocatedResponse(csiResp *csi.GetMetadataAllocatedResponse) *api.GetMetadataAllocatedResponse {
	apiResp := &api.GetMetadataAllocatedResponse{
		BlockMetadataType:   api.BlockMetadataType(csiResp.BlockMetadataType),
		VolumeCapacityBytes: csiResp.VolumeCapacityBytes,
	}

	for _, b := range csiResp.GetBlockMetadata() {
		apiResp.BlockMetadata = append(apiResp.BlockMetadata, &api.BlockMetadata{
			ByteOffset: b.ByteOffset,
			SizeBytes:  b.SizeBytes,
		})
	}

	return apiResp
}
