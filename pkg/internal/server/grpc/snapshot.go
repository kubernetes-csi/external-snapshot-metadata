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

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type volSnapshotInfo struct {
	DriverName                string
	SnapshotHandle            string
	SourceVolume              string
	VolumeSnapshot            *snapshotv1.VolumeSnapshot
	VolumeSnapshotContentName string
}

// getVolSnapshotInfo returns the CSI snapshot handle and driver name of the specified VolumeSnapshot.
func (s *Server) getVolSnapshotInfo(ctx context.Context, namespace, vsName string) (*volSnapshotInfo, error) {
	vs, err := s.getVolumeSnapshot(ctx, namespace, vsName)
	if err != nil {
		return nil, err
	}

	sourceVolume := ""
	if vs.Spec.Source.PersistentVolumeClaimName != nil {
		sourceVolume = *vs.Spec.Source.PersistentVolumeClaimName
	}

	vsc, err := s.getVolumeSnapshotContent(ctx, *vs.Status.BoundVolumeSnapshotContentName, vsName)
	if err != nil {
		return nil, err
	}

	return &volSnapshotInfo{
		DriverName:                vsc.Spec.Driver,
		SnapshotHandle:            *vsc.Status.SnapshotHandle,
		SourceVolume:              sourceVolume,
		VolumeSnapshot:            vs,
		VolumeSnapshotContentName: vsc.Name,
	}, nil
}

func (s *Server) getVolumeSnapshot(ctx context.Context, namespace, vsName string) (*snapshotv1.VolumeSnapshot, error) {
	vs, err := s.snapshotClient().SnapshotV1().VolumeSnapshots(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableFailedToGetVolumeSnapshot, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableFailedToGetVolumeSnapshotFmt, namespace, vsName, err)
	}

	if vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		klog.FromContext(ctx).Error(err, msgUnavailableVolumeSnapshotNotReady, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableVolumeSnapshotNotReadyFmt, vsName)
	}

	if vs.Status.BoundVolumeSnapshotContentName == nil {
		klog.FromContext(ctx).Error(err, msgUnavailableInvalidVolumeSnapshotStatus, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableInvalidVolumeSnapshotStatusFmt, vsName)
	}

	return vs, nil
}

func (s *Server) getVolumeSnapshotContent(ctx context.Context, vscName, vsName string) (*snapshotv1.VolumeSnapshotContent, error) {
	vsc, err := s.snapshotClient().SnapshotV1().VolumeSnapshotContents().Get(ctx, vscName, metav1.GetOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableFailedToGetVolumeSnapshotContent, "vscName", vscName, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableFailedToGetVolumeSnapshotContentFmt, vscName, err)
	}

	if vsc.Status.ReadyToUse == nil || !*vsc.Status.ReadyToUse {
		klog.FromContext(ctx).Error(err, msgUnavailableVolumeSnapshotContentNotReady, "vscName", vscName, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableVolumeSnapshotContentNotReadyFmt, vscName)
	}

	if vsc.Status.SnapshotHandle == nil {
		klog.FromContext(ctx).Error(err, msgUnavailableInvalidVolumeSnapshotContentStatus, "vscName", vscName, "vsName", vsName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableInvalidVolumeSnapshotContentStatusFmt, vscName)
	}

	return vsc, nil
}
