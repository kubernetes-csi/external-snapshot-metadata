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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func (s *Server) registerService() {
	api.RegisterSnapshotMetadataServer(s.grpcServer, s)
}

func (s *Server) GetMetadataAllocated(req *api.GetMetadataAllocatedRequest, stream api.SnapshotMetadata_GetMetadataAllocatedServer) error {
	// validate request
	if len(req.GetSecurityToken()) == 0 {
		return status.Errorf(codes.InvalidArgument, "securityToken is missing")
	}
	if len(req.GetNamespace()) == 0 {
		return status.Errorf(codes.InvalidArgument, "namespace parameter cannot be empty")
	}
	if len(req.GetSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, "snapshotName cannot be empty")
	}
	// TODO: Add requst authn/authz and business logic
	return nil
}
func (s *Server) GetMetadataDelta(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) error {
	// validate request
	if len(req.GetSecurityToken()) == 0 {
		return status.Errorf(codes.InvalidArgument, "securityToken is missing")
	}
	if len(req.GetNamespace()) == 0 {
		return status.Errorf(codes.InvalidArgument, "namespace parameter cannot be empty")
	}
	if len(req.GetBaseSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, "baseSnapshotName cannot be empty")
	}
	if len(req.GetTargetSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, "targetSnapshotName cannot be empty")
	}
	// TODO: Add requst authn/authz and business logic
	return nil
}
