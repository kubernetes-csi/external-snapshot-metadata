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
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) GetMetadataAllocated(req *api.GetMetadataAllocatedRequest, stream api.SnapshotMetadata_GetMetadataAllocatedServer) error {
	if err := ValidateGetMetadataAllocatedRequest(req); err != nil {
		return err
	}

	ctx := stream.Context()
	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	// TODO: Call CSI driver endpoint for changed block metadata
	return nil
}

func (s *Server) GetMetadataDelta(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) error {
	if err := ValidateGetMetadataDeltaRequest(req); err != nil {
		return err
	}

	ctx := stream.Context()
	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(); err != nil {
		return err
	}

	// TODO: Call CSI driver endpoint for changed block metadata
	return nil
}

// isCSIDriverReady is a helper for the handlers that returns the appropriate error if the
// CSI driver is not ready.
func (s *Server) isCSIDriverReady() error {
	if s.isReady() {
		return nil
	}

	return status.Errorf(codes.Unavailable, msgUnavailableCSIDriverNotReady)
}
