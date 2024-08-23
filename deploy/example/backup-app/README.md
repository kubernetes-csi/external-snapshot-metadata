# Authorizing a Backup Application

This document provides steps to authorize a backup application to interact with the snapshot-metadata service and utilize the Changed Block Tracking (CBT) feature. You can use this example as a reference when deploying the backup application with the required permissions to use the CBT feature.

## Prerequisites:

- Installed ClusterRoles and CRDs: Follow the instructions in this [guide](../../README.md) to ensure these components are installed.

## Installation

In this example, we will create the Kubernetes RBAC configuration for a backup application so that it can communicate with the snapshot-metadata service to retrieve CBT metadata for the given CSI snapshots.

**Steps to deploy the sample backup application**

1. Create a namespace

   ```bash
   $ kubectl create namespace backup-app-namespace
   ```

2. Create a ServiceAccount for the backup application

   ```bash
   $ kubectl create -f service-account.yaml
   ```

3. Create a ClusterRoleBinidng to authorize the backp application's ServiceAccount

   ```bash
   $ kubectl create -f cluster-role-binding.yaml
   ```