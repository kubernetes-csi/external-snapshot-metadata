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

func (s *Server) GetMetadataDelta(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) error {
	ctx := stream.Context()

	if err := s.validateGetMetadataDeltaRequest(req); err != nil {
		return err
	}

	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	csiReq, err := s.convertToCSIGetMetadataDeltaRequest(ctx, req)
	if err != nil {
		return err
	}

	// Invoke the CSI Driver's GetMetadataDelta gRPC and stream the response back to client
	csiStream, err := csi.NewSnapshotMetadataClient(s.csiConnection()).GetMetadataDelta(ctx, csiReq)
	if err != nil {
		return err
	}

	return s.streamGetMetadataDeltaResponse(stream, csiStream)
}

func (s *Server) validateGetMetadataDeltaRequest(req *api.GetMetadataDeltaRequest) error {
	if len(req.GetSecurityToken()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentSecurityTokenMissing)
	}

	if len(req.GetNamespace()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentNamespaceMissing)
	}

	if len(req.GetBaseSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentBaseSnapshotNameMissing)
	}

	if len(req.GetTargetSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentTargetSnapshotNameMissing)
	}

	return nil
}

func (s *Server) convertToCSIGetMetadataDeltaRequest(ctx context.Context, req *api.GetMetadataDeltaRequest) (*csi.GetMetadataDeltaRequest, error) {
	vsiBase, err := s.getVolSnapshotInfo(ctx, req.Namespace, req.BaseSnapshotName)
	if err != nil {
		return nil, err
	}

	if vsiBase.DriverName != s.driverName() {
		return nil, status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, req.BaseSnapshotName, s.driverName())
	}

	vsiTarget, err := s.getVolSnapshotInfo(ctx, req.Namespace, req.TargetSnapshotName)
	if err != nil {
		return nil, err
	}

	if vsiTarget.DriverName != s.driverName() {
		return nil, status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, req.TargetSnapshotName, s.driverName())
	}

	if vsiBase.SourceVolume != vsiTarget.SourceVolume {
		return nil, status.Errorf(codes.InvalidArgument, msgInvalidArgumentDiffSnapshotSourceVolumes)
	}

	// the target was created after the base so use its secrets.
	secretsMap, err := s.getSnapshotterCredentials(ctx, vsiTarget)
	if err != nil {
		return nil, err
	}

	return &csi.GetMetadataDeltaRequest{
		BaseSnapshotId:   vsiBase.SnapshotHandle,
		TargetSnapshotId: vsiTarget.SnapshotHandle,
		StartingOffset:   req.StartingOffset,
		MaxResults:       req.MaxResults,
		Secrets:          secretsMap,
	}, nil
}

func (s *Server) streamGetMetadataDeltaResponse(clientStream api.SnapshotMetadata_GetMetadataDeltaServer, csiStream csi.SnapshotMetadata_GetMetadataDeltaClient) error {
	for {
		csiResp, err := csiStream.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return s.statusPassOrWrapError(err, codes.Internal, msgInternalFailedCSIDriverResponseFmt, err)
		}

		clientResp := s.convertToGetMetadataDeltaResponse(csiResp)
		if err := clientStream.Send(clientResp); err != nil {
			return s.statusPassOrWrapError(err, codes.Internal, msgInternalFailedtoSendResponseFmt, err)
		}
	}
}

func (s *Server) convertToGetMetadataDeltaResponse(csiResp *csi.GetMetadataDeltaResponse) *api.GetMetadataDeltaResponse {
	apiResp := &api.GetMetadataDeltaResponse{
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
