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
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func TestApplyMetadataToGetMetadataAllocatedRequest(t *testing.T) {
	t.Run("fills-empty-fields", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			metadataSecurityTokenKey, "token-1",
			metadataNamespaceKey, "ns-1",
			metadataSnapshotNameKey, "snap-1",
		))

		req := &api.GetMetadataAllocatedRequest{}
		applyMetadataToGetMetadataAllocatedRequest(ctx, req)

		assert.Equal(t, "token-1", req.SecurityToken)
		assert.Equal(t, "ns-1", req.Namespace)
		assert.Equal(t, "snap-1", req.SnapshotName)
	})

	t.Run("does-not-override-existing-fields", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			metadataSecurityTokenKey, "token-2",
			metadataNamespaceKey, "ns-2",
			metadataSnapshotNameKey, "snap-2",
		))

		req := &api.GetMetadataAllocatedRequest{
			SecurityToken: "token-req",
			Namespace:     "ns-req",
			SnapshotName:  "snap-req",
		}
		applyMetadataToGetMetadataAllocatedRequest(ctx, req)

		assert.Equal(t, "token-req", req.SecurityToken)
		assert.Equal(t, "ns-req", req.Namespace)
		assert.Equal(t, "snap-req", req.SnapshotName)
	})
}

func TestApplyMetadataToGetMetadataDeltaRequest(t *testing.T) {
	t.Run("fills-empty-fields", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			metadataSecurityTokenKey, "token-1",
			metadataNamespaceKey, "ns-1",
			metadataBaseSnapshotIDKey, "base-1",
			metadataTargetSnapshotNameKey, "snap-1",
		))

		req := &api.GetMetadataDeltaRequest{}
		applyMetadataToGetMetadataDeltaRequest(ctx, req)

		assert.Equal(t, "token-1", req.SecurityToken)
		assert.Equal(t, "ns-1", req.Namespace)
		assert.Equal(t, "base-1", req.BaseSnapshotId)
		assert.Equal(t, "snap-1", req.TargetSnapshotName)
	})

	t.Run("does-not-override-existing-fields", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			metadataSecurityTokenKey, "token-2",
			metadataNamespaceKey, "ns-2",
			metadataBaseSnapshotIDKey, "base-2",
			metadataTargetSnapshotNameKey, "snap-2",
		))

		req := &api.GetMetadataDeltaRequest{
			SecurityToken:      "token-req",
			Namespace:          "ns-req",
			BaseSnapshotId:     "base-req",
			TargetSnapshotName: "snap-req",
		}
		applyMetadataToGetMetadataDeltaRequest(ctx, req)

		assert.Equal(t, "token-req", req.SecurityToken)
		assert.Equal(t, "ns-req", req.Namespace)
		assert.Equal(t, "base-req", req.BaseSnapshotId)
		assert.Equal(t, "snap-req", req.TargetSnapshotName)
	})
}
