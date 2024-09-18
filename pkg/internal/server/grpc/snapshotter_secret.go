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
	snapshotutils "github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// getSnapshotterCredentials returns the snapshotter secret associated with VolumeSnapshotClass, if any.
// The VolumeSnapshotClass is fetched by name if specified, or else the default VolumeSnapshotClass
// for the CSI driver is used.
func (s *Server) getSnapshotterCredentials(ctx context.Context, vsi *volSnapshotInfo) (map[string]string, error) {
	secretRef, err := s.getSnapshotterSecretRef(ctx, vsi)
	if err != nil {
		return nil, err
	}

	if secretRef == nil {
		return nil, nil
	}

	// snapshotutils.GetCredentials gets the secrets for the external-snapshotter CSI calls.
	secretMap, err := snapshotutils.GetCredentials(s.kubeClient(), secretRef)
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableFailedToGetCredentials)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableFailedToGetCredentialsFmt, err) // error contains secret ref
	}

	return secretMap, nil
}

// getSnapshotterSecretRef returns the name of the snapshotter secret from a VolumeSnapshotClass.
// The VolumeSnapshotClass is fetched by name if specified, or else the default VolumeSnapshotClass
// for the CSI driver is used.
// On successful return, either both secretNs and secretName are set or neither are set.
func (s *Server) getSnapshotterSecretRef(ctx context.Context, vsi *volSnapshotInfo) (*corev1.SecretReference, error) {
	var vsClass *snapshotv1.VolumeSnapshotClass
	var err error

	if vsi.VolumeSnapshot.Spec.VolumeSnapshotClassName != nil {
		vsClass, err = s.getVolumeSnapshotClass(ctx, *vsi.VolumeSnapshot.Spec.VolumeSnapshotClassName)
	} else {
		vsClass, err = s.getDefaultVolumeSnapshotClassForDriver(ctx, vsi.DriverName)
	}

	if err != nil {
		return nil, err
	}

	// snapshotutils.GetSecretReference handles template substitution in name and namespace in the external-snapshotter.
	secretRef, err := snapshotutils.GetSecretReference(snapshotutils.SnapshotterSecretParams, vsClass.Parameters, vsi.VolumeSnapshotContentName, vsi.VolumeSnapshot)
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableInvalidSecretInVolumeSnapshotClass, "volumeSnapshotClassName", vsClass.Name)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableInvalidSecretInVolumeSnapshotClassFmt, err) // error describes what could be wrong
	}

	return secretRef, nil
}

func (s *Server) getVolumeSnapshotClass(ctx context.Context, volumeSnapshotClassName string) (*snapshotv1.VolumeSnapshotClass, error) {
	vsc, err := s.snapshotClient().SnapshotV1().VolumeSnapshotClasses().Get(ctx, volumeSnapshotClassName, metav1.GetOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableFailedToGetVolumeSnapshotClass, "volumeSnapshotClassName", volumeSnapshotClassName)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableFailedToGetVolumeSnapshotClassFmt, volumeSnapshotClassName, err)
	}

	return vsc, nil
}

func (s *Server) getDefaultVolumeSnapshotClassForDriver(ctx context.Context, driverName string) (*snapshotv1.VolumeSnapshotClass, error) {
	vscList, err := s.snapshotClient().SnapshotV1().VolumeSnapshotClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, msgUnavailableFailedToListVolumeSnapshotClasses)
		return nil, status.Errorf(codes.Unavailable, msgUnavailableFailedToListVolumeSnapshotClassesFmt, err)
	}

	idxFound := -1

	for idx, vsc := range vscList.Items {
		if vsc.Driver != driverName || vsc.Annotations == nil {
			continue
		}

		annValue, found := vsc.Annotations[snapshotutils.IsDefaultSnapshotClassAnnotation]
		if !found {
			continue
		}

		if annValue == "true" {
			if idxFound != -1 { // it is an error if there are multiple defaults
				klog.FromContext(ctx).Error(err, msgUnavailableMultipleDefaultVolumeSnapshotClassesForDriver, "driver", driverName)
				return nil, status.Errorf(codes.Unavailable, msgUnavailableMultipleDefaultVolumeSnapshotClassesForDriverFmt, driverName)
			}

			idxFound = idx
		}
	}

	if idxFound != -1 {
		return &vscList.Items[idxFound], nil
	}

	klog.FromContext(ctx).Error(err, msgUnavailableNoDefaultVolumeSnapshotClassForDriver, "driver", driverName)
	return nil, status.Errorf(codes.Unavailable, msgUnavailableNoDefaultVolumeSnapshotClassForDriverFmt, driverName)
}
