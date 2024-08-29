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
	"io"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) streamGetMetadataDeltaResponse(clientStream api.SnapshotMetadata_GetMetadataDeltaServer, csiStream csi.SnapshotMetadata_GetMetadataDeltaClient) error {
	for {
		csiResp, err := csiStream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, msgInternalFailedCSIDriverResponseFmt, err)
		}
		clientResp := convertToGetMetadataDeltaResponse(csiResp)
		if err := clientStream.Send(clientResp); err != nil {
			return status.Errorf(codes.Internal, msgInternalFailedtoSendResponseFmt, err)
		}
	}
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
		clientResp := convertToGetMetadataAllocatedResponse(csiResp)
		if err := clientStream.Send(clientResp); err != nil {
			return status.Errorf(codes.Internal, msgInternalFailedtoSendResponseFmt, err)
		}
	}
}
