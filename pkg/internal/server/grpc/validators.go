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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
)

func (s *Server) ValidateGetMetadataAllocatedRequest(ctx context.Context, req *api.GetMetadataAllocatedRequest) (string, error) {
	if len(req.GetSecurityToken()) == 0 {
		return "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSecurityTokenMissing)
	}

	if len(req.GetNamespace()) == 0 {
		return "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentNamespaceMissing)
	}

	if len(req.GetSnapshotName()) == 0 {
		return "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotNameMissing)
	}

	snapshotHandle, driver, err := getVolSnapshotInfo(ctx, s.snapshotClient(), req.Namespace, req.SnapshotName)
	if err != nil {
		return "", err
	}

	if driver != s.driverName() {
		return "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, s.driverName())
	}

	return snapshotHandle, nil
}

func (s *Server) ValidateGetMetadataDeltaRequest(ctx context.Context, req *api.GetMetadataDeltaRequest) (string, string, error) {
	if len(req.GetSecurityToken()) == 0 {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSecurityTokenMissing)
	}

	if len(req.GetNamespace()) == 0 {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentNamespaceMissing)
	}

	if len(req.GetBaseSnapshotName()) == 0 {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentBaseSnapshotNameMissing)
	}

	if len(req.GetTargetSnapshotName()) == 0 {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentTargetSnapshotNameMissing)
	}

	baseSnapshotHandle, driver, err := getVolSnapshotInfo(ctx, s.snapshotClient(), req.Namespace, req.BaseSnapshotName)
	if err != nil {
		return "", "", err
	}

	if driver != s.driverName() {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, s.driverName())
	}

	targetSnapshotHandle, driver, err := getVolSnapshotInfo(ctx, s.snapshotClient(), req.Namespace, req.TargetSnapshotName)
	if err != nil {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableVolumeSnapshotNotReady)
	}

	if driver != s.driverName() {
		return "", "", status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, s.driverName())
	}

	return baseSnapshotHandle, targetSnapshotHandle, nil
}

// getVolSnapshotInfo returns snapshot handle and csi driver info of the VolumeSnapshot.
func getVolSnapshotInfo(ctx context.Context, cli snapshot.Interface, namespace, vsName string) (string, string, error) {
	vs, err := cli.SnapshotV1().VolumeSnapshots(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableFailedToGetVolumeSnapshotFmt, err)
	}
	if vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableVolumeSnapshotNotReadyFmt, vsName)
	}
	if vs.Status.BoundVolumeSnapshotContentName == nil {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableInvalidVolumeSnapshotStatusFmt, vsName)
	}

	vsc, err1 := cli.SnapshotV1().VolumeSnapshotContents().Get(ctx, *vs.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
	if err1 != nil {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableFailedToGetVolumeSnapshotContentFmt, err)
	}
	if vsc.Status.ReadyToUse == nil || !*vsc.Status.ReadyToUse {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableVolumeSnapshotContentNotReadyFmt, vsc.Name)
	}
	if vsc.Status.SnapshotHandle == nil {
		return "", "", status.Errorf(codes.Unavailable, msgUnavailableInvalidVolumeSnapshotContentStatusFmt, vsName)
	}
	return *vsc.Status.SnapshotHandle, vsc.Spec.Driver, nil
}
