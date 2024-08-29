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

const (
	mgsInternalFailedToAuthorizePrefix    = "failed to authorize the user"
	mgsInternalFailedToAuthorizeFmt       = mgsInternalFailedToAuthorizePrefix + ": %v"
	msgInternalFailedToAuthenticatePrefix = "failed to authenticate user"
	msgInternalFailedToAuthenticateFmt    = msgInternalFailedToAuthenticatePrefix + ": %v"
	msgInternalFailedCSIDriverResponse    = "failed to get response from CSI driver"
	msgInternalFailedCSIDriverResponseFmt = msgInternalFailedCSIDriverResponse + ": %v"
	msgInternalFailedToSendResponse       = "failed to send response"
	msgInternalFailedtoSendResponseFmt    = msgInternalFailedToSendResponse + ": %v"

	msgInvalidArgumentBaseSnapshotNameMissing   = "baseSnapshotName cannot be empty"
	msgInvalidArgumentNamespaceMissing          = "namespace parameter cannot be empty"
	msgInvalidArgumentSecurityTokenMissing      = "securityToken is missing"
	msgInvalidArgumentSnaphotNameMissing        = "snapshotName cannot be empty"
	msgInvalidArgumentTargetSnapshotNameMissing = "targetSnapshotName cannot be empty"
	msgInvalidArgumentSnaphotDriverInvalidFmt   = "VolumeSnapshot does not belong to the CSI driver %s"

	msgPermissionDeniedPrefix = "user does not have permissions to perform the operation"
	msgPermissionDeniedFmt    = msgPermissionDeniedPrefix + ": %s"

	msgUnauthenticatedUser = "unauthenticated user"

	msgUnavailableCSIDriverNotReady = "the CSI driver is not yet ready"

	msgUnavailableFailedToGetVolumeSnapshot      = "failed to get VolumeSnapshot"
	msgUnavailableFailedToGetVolumeSnapshotFmt   = msgUnavailableFailedToGetVolumeSnapshot + ": %v"
	msgUnavailableVolumeSnapshotNotReady         = "the VolumeSnapshot is not yet ready"
	msgUnavailableVolumeSnapshotNotReadyFmt      = msgUnavailableVolumeSnapshotNotReady + ", name: %s"
	msgUnavailableInvalidVolumeSnapshotStatus    = "boundVolumeSnapshotContentName is not set in VolumeSnapshot status"
	msgUnavailableInvalidVolumeSnapshotStatusFmt = msgUnavailableInvalidVolumeSnapshotStatus + "name: %s"

	msgUnavailableFailedToGetVolumeSnapshotContent      = "failed to get VolumeSnapshotContent"
	msgUnavailableFailedToGetVolumeSnapshotContentFmt   = msgUnavailableFailedToGetVolumeSnapshotContent + ": %v"
	msgUnavailableVolumeSnapshotContentNotReady         = "the VolumeSnapshotContent is not yet ready"
	msgUnavailableVolumeSnapshotContentNotReadyFmt      = msgUnavailableVolumeSnapshotContentNotReady + ", name: %s"
	msgUnavailableInvalidVolumeSnapshotContentStatus    = "snapshotHandle is not set in VolumeSnapshotContent status"
	msgUnavailableInvalidVolumeSnapshotContentStatusFmt = msgUnavailableInvalidVolumeSnapshotContentStatus + "name: %s"
)
