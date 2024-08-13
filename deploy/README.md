# Deployment guide

## Prerequisites:

Please ensure that your environment meets the following minimum requirements to use the Changed Block Tracking (CBT) feature:

- Kubernetes 1.20 or higher
- CSI provisioner with Volume Snapshot and SnapshotMetadata support.


## Installation

This directory contains the following components:

**1. Common components**

  This includes the necessary Roles, ClusterRoles, and Custom Resource Definitions (CRDs) that must be installed on your Kubernetes cluster independent of the CSI driver prior to deploying any CSI driver that utilizes the external-snapshot-metadata sidecar.

**2. CSI Driver Example**

  The `examples/csi-driver` directory contains sample manifests for deploying the external-snapshot-metadata sidecar with a CSI driver. You can use this example as a reference when installing a CSI driver with snapshot metadata support in your cluster.

**3. Backup Application Example**

The `examples/backup-app` directory provides sample manifests for Role-Based Access Control (RBAC) resources and a sample backup application. This example demonstrates how to set up a backup application that communicates with the snapshot-metadata service to utilize the Changed Block Tracking (CBT) feature.

### Installation Steps

Before installing the snapshot-metadata service, you must create specific Roles, ClusterRoles, and CRDs that are required for the service to function within your Kubernetes clusters.
These components are independent of any CSI driver you intend to install.

1. Create the ClusterRole for external-snapshot-metadata service.

   Grant the necessary permissions to the snapshot-metadata service by creating the required ClusterRole:
   
   ```bash
   $ kubectl create -f snapshot-metadata-cluster-role.yaml
   ```

2. Create the ClusterRole for snapshot-metadata client.

   Grant the necessary permissions to the snapshot-metadata client by creating the required ClusterRole:
   
   ```bash
   $ kubectl create -f snapshot-metadata-client-cluster-role.yaml
   ```

3. Create CRD

   Register the SnapshotMetadataService resource by creating the necessary CRD:

   ```bash
   $ kubectl create -f cbt.storage.k8s.io_snapshotmetadataservices.yaml
   ```

### Next Steps

Refer to the `examples/csi-driver` and `examples/backup-app` directories to deploy the snapshot-metadata service and the backup application.