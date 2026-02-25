/*
Copyright 2026 The Kubernetes Authors.

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

	"google.golang.org/grpc/metadata"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

const (
	metadataSecurityTokenKey      = "security_token"
	metadataNamespaceKey          = "namespace"
	metadataBaseSnapshotIDKey     = "base_snapshot_id"
	metadataSnapshotNameKey       = "snapshot_name"
	metadataTargetSnapshotNameKey = "target_snapshot_name"
)

func metadataValue(ctx context.Context, key string) string {
	if ctx == nil || key == "" {
		return ""
	}

	values := metadata.ValueFromIncomingContext(ctx, key)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}

func applyMetadataToGetMetadataAllocatedRequest(ctx context.Context, req *api.GetMetadataAllocatedRequest) {
	if req == nil {
		return
	}

	if req.SecurityToken == "" {
		req.SecurityToken = metadataValue(ctx, metadataSecurityTokenKey)
	}

	if req.Namespace == "" {
		req.Namespace = metadataValue(ctx, metadataNamespaceKey)
	}

	if req.SnapshotName == "" {
		req.SnapshotName = metadataValue(ctx, metadataSnapshotNameKey)
	}
}

func applyMetadataToGetMetadataDeltaRequest(ctx context.Context, req *api.GetMetadataDeltaRequest) {
	if req == nil {
		return
	}

	if req.SecurityToken == "" {
		req.SecurityToken = metadataValue(ctx, metadataSecurityTokenKey)
	}

	if req.Namespace == "" {
		req.Namespace = metadataValue(ctx, metadataNamespaceKey)
	}

	if req.BaseSnapshotId == "" {
		req.BaseSnapshotId = metadataValue(ctx, metadataBaseSnapshotIDKey)
	}

	if req.TargetSnapshotName == "" {
		req.TargetSnapshotName = metadataValue(ctx, metadataTargetSnapshotNameKey)
	}
}
