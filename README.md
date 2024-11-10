# external-snapshot-metadata

This repository contains components that implement the
[KEP-3314 CSI Changed Block Tracking](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/3314-csi-changed-block-tracking)
feature.

## Overview

The repository provides the following components:

- The [External Snapshot Metadata sidecar](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/3314-csi-changed-block-tracking#the-external-snapshot-metadata-sidecar)
  container that exposes the
  [Kubernetes SnapshotMetadataService API](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/3314-csi-changed-block-tracking#the-kubernetes-snapshotmetadata-service-api)
  to backup applications, and interacts with a CSI driver's implementation
  of the
  [CSI SnapshotMetadataService](https://github.com/container-storage-interface/spec/blob/master/spec.md#snapshot-metadata-service-rpcs).
- The [SnapshotMetadataService CRD](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/client/config/crd)
  used to advertise the CSI Driver's support for the
  CSI Changed Block Tracking feature.
- gRPC stubs
  - [Kubernetes SnapshotMetadataService API](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/pkg/api)
  - [Mocks for the Kubernetes SnapshotMetadataService API client](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/pkg/k8sclientmocks)
  - [Mocks for the CSI SnapshotMetadataService API client](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/pkg/csiclientmocks)
- Examples
  - [Instructions on how to deploy a CSI driver with the `external-snapshot-metadata` sidecar](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/main/deploy).
  - A [snapshot-metadata-lister](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/examples/snapshot-metadata-lister)
  example command to illustrate how a Kubernetes backup application could fetch snapshot metadata.
    This is based on [pkg/iterator](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/master/pkg/iterator),
    which a backup application developer could use directly to fetch snapshot metadata, if desired.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing List](https://groups.google.com/g/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
