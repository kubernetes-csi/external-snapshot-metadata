# Example snapshot-metadata Service with CSI Driver

This document illustrates how to install a CSI driver with the external-snapshot-metadata sidecar. You can use this example as a reference when installing a CSI driver with Changed Block Metadata (CBT) support in your cluster.

## Prerequisites:

Ensure that you have installed the necessary ClusterRoles and CRDs as explained in this [guide](../../README.md).


## Installation

In this example, we will deploy the snapshot-metadata service alongside a dummy CSI driver. While this example uses a dummy CSI driver, the steps may vary depending on the specific CSI driver you are using. Use the appropriate steps to deploy the CSI driver in your environment.

**Steps to deploy snapshot-metadata with a dummy CSI driver:**

1. Create a namespace

   ```bash
   $ kubectl create namespace csi-driver
   ```
   If you prefer to use different namespace, update the `namespace` fields in `csi-driver-with-snapshot-metadata-sidecar.yaml`.

2. Provision TLS Certs

   Generate self-signed certificates using following commands

   ```bash
   NAMESPACE="csi-driver"

   # 1. Create extension file
   echo "subjectAltName=DNS:.csi-driver,DNS:csi-dummyplugin.csi-driver,DNS:csi-dummyplugin.default,IP:0.0.0.0" > server-ext.cnf 

   # 2. Generate CA's private key and self-signed certificate
   openssl req -x509 -newkey rsa:4096 -days 365 -nodes -keyout ca-key.pem -out ca-cert.pem -subj "/CN=csi-dummyplugin.${NAMESPACE}"

   openssl x509 -in ca-cert.pem -noout -text
   
   # 2. Generate web server's private key and certificate signing request (CSR)
   openssl req -newkey rsa:4096 -nodes -keyout server-key.pem -out server-req.pem -subj "/CN=csi-dummyplugin.${NAMESPACE}"
   
   # 3. Use CA's private key to sign web server's CSR and get back the signed certificate
   openssl x509 -req -in server-req.pem -days 60 -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial -out server-cert.pem -extfile server-ext.cnf
   
   openssl x509 -in server-cert.pem -noout -text
   ```

3. Create a TLS secret 

   ```bash
   $ kubectl create secret tls csi-dummyplugin-certs --namespace=csi-driver --cert=server-cert.pem --key=server-key.pem 
   ```

4. Create `SnapshotMetadataService` resource

   The name of the `SnapshotMetadataService` resource must match the name of the CSI driver for which you want to enable the CBT feature. In this example, we will create a `SnapshotMetadataService` for the `dummy.csi.k8s.io` CSI driver.

   Create a file named `snapshotmetadataservice.yaml` with the following content:

   ```yaml
   apiVersion: cbt.storage.k8s.io/v1alpha1
   kind: SnapshotMetadataService
   metadata:
     name: dummy.csi.k8s.io
   spec:
     address: csi-dummyplugin.csi-driver:6443
     caCert: GENERATED_CA_CERT
     audience: 005e2583-91a3-4850-bd47-4bf32990fd00
   ```

   Encode the CA Cert:

   ```bash
   $ base64 -i ca-cert.pem
   ```

   Copy the output and replace `GENERATED_CA_CERT` in the `SnapshotMetadataService` CR.
   Update `spec.address` and `spec.audience` if required.

   Create `SnapshotMetadataService` resource using the command below:

   ```bash
   $ kubectl create -f snapshotmetadataservice.yaml
   ```

5. Create ServiceAccount and ClusterRoleBinding for snapshot-metadata service

   ```bash
   $ kubectl create -f csi-driver-service-account.yaml
   $ kubectl create -f csi-driver-cluster-role-binding.yaml
   ```

6. Deploy the CSI driver with snapshot-metadata sidecar service

   ```bash
   $ kubectl create -f csi-driver-with-snapshot-metadata-sidecar.yaml --namespace csi-driver
   ```