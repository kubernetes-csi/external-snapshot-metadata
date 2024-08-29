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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func newCSIGetMetadataDeltaRequest(ctx context.Context, baseSnapHandle, targetSnapHandle string, req *api.GetMetadataDeltaRequest) *csi.GetMetadataDeltaRequest {
	return &csi.GetMetadataDeltaRequest{
		BaseSnapshotId:   baseSnapHandle,
		TargetSnapshotId: targetSnapHandle,
		StartingOffset:   req.StartingOffset,
		MaxResults:       req.MaxResults,
		//TODO: set Secrets field
	}
}

func newCSIGetMetadataAllocatedRequest(ctx context.Context, snapshotHandle string, req *api.GetMetadataAllocatedRequest) *csi.GetMetadataAllocatedRequest {
	return &csi.GetMetadataAllocatedRequest{
		SnapshotId:     snapshotHandle,
		StartingOffset: req.StartingOffset,
		MaxResults:     req.MaxResults,
		//TODO: set Secrets field
	}
}

func convertToGetMetadataDeltaResponse(csiResp *csi.GetMetadataDeltaResponse) *api.GetMetadataDeltaResponse {
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

func convertToGetMetadataAllocatedResponse(csiResp *csi.GetMetadataAllocatedResponse) *api.GetMetadataAllocatedResponse {
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
