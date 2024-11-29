#! /bin/bash

# Copyright 2024 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e
set -x

SCRIPT_DIR="$(dirname "${0}")"

# shellcheck disable=SC1091
[ ! -e "${SCRIPT_DIR}"/utils.sh ] || . "${SCRIPT_DIR}"/utils.sh

TEMP_DIR="$(mktemp -d)"
# trap 'rm -rf ${TEMP_DIR}' EXIT

# snapshot metadata CRDs
SNAPSHOT_METADATA_SERVICE_CRD="./client/config/crd/cbt.storage.k8s.io_snapshotmetadataservices.yaml"

SNAPSHOT_METADATA_CLUSTER_ROLE="./deploy/snapshot-metadata-cluster-role.yaml"
SNAPSHOT_METADATA_CLUSTER_ROLE_BINDING="./deploy/example/csi-driver/csi-driver-cluster-role-binding.yaml"
SNAPSHOT_METADATA_DRIVER_SERVICE="./deploy/example/csi-driver/csi-driver-service.yaml"
SNAPSHOT_METADATA_SERVICE="./deploy/example/csi-driver/snapshotmetadataservice.yaml"
SNAPSHOT_METADATA_CERTS_SECRET="./deploy/example/csi-driver/snapshot-metadata-certs-secret.yaml"

SNAPSHOT_METADATA_CLIENT_CLUSTER_ROLE="./deploy/snapshot-metadata-client-cluster-role.yaml"
SNAPSHOT_METADATA_CLIENT_CLUSTER_ROLEBINDING="./deploy/backup-app/snapshot-metadata-client-cluster-rolebinding.yaml"
SNAPSHOT_METADATA_CLIENT_POD="./deploy/backup-app/snapshot-metadata-client-pod.yaml"

NAMESPACE="default"

function create_or_delete_crds() {
    local action=$1
    kubectl_retry "${action}" -f "${SNAPSHOT_METADATA_SERVICE_CRD}"
}

function create_or_delete_tls_certs() {
    local action=$1
    if [ "${action}" == "delete" ]; then
        kubectl_retry "${action}" secret csi-snapshot-metadata-certs --namespace="${NAMESPACE}"
        return 0
    fi
    kubectl_retry "${action}" -f "${SNAPSHOT_METADATA_CERTS_SECRET}"\
        --namespace="${NAMESPACE}" 
}


function create_or_delete_rbacs() {
    local action=$1
    temp_file=$(mktemp "${TEMP_DIR}/snapshot-metadata-rolebinding.XXXXXX.yaml")
    cp "${SNAPSHOT_METADATA_CLUSTER_ROLE_BINDING}" "${temp_file}"
    namespace=$NAMESPACE yq -i ".subjects[0].namespace = env(namespace)" "${temp_file}"
    yq -i '.subjects[0].name = "csi-hostpathplugin-sa"' "${temp_file}"

    kubectl_retry "${action}" -f "${SNAPSHOT_METADATA_CLUSTER_ROLE}"
    kubectl_retry "${action}" -f "${temp_file}"
}

function create_or_delete_snapshot_metadata_service() {
    local action=$1

    kubectl_retry "${action}" -f "${SNAPSHOT_METADATA_SERVICE}"
}


function patch_snapshot_metadata_sidecar() {
    kubectl get statefulset csi-hostpathplugin -oyaml > hostplugin.yaml
    yq -i '
        .spec.template.spec.containers += [
            {
                "name": "csi-snapshot-metadata",
                "image": "gcr.io/k8s-staging-sig-storage/csi-snapshot-metadata:test",
                "imagePullPolicy": "IfNotPresent",
                "args": [
                    "--v=5",
                    "--port=50051",
                    "--csi-address=/csi/csi.sock",
                    "--tls-cert=/tmp/certificates/tls.crt",
                    "--tls-key=/tmp/certificates/tls.key"
                ],
                "volumeMounts": [
                    {
                        "mountPath": "/csi",
                        "name": "socket-dir"
                    },
                    {
                        "mountPath": "/tmp/certificates",
                        "name": "csi-snapshot-metadata-certs",
                        "readOnly": true
                    }
                ]
            }
        ] | 
        .spec.template.spec.volumes += [
            {
                "name": "csi-snapshot-metadata-certs",
                "secret": {
                    "secretName": "csi-snapshot-metadata-certs"
                }
            }
        ]
    ' hostplugin.yaml

    # enable snapshot-metadata capability in hostpath plugin
    yq -i '
        .spec.template.spec.containers[] |=
        select(.name == "hostpath") |=
        .args += "--enable-snapshot-metadata=true"
    ' hostplugin.yaml

    # Delete and recreate the csi-hostpathplugin statefulset
    kubectl delete -f hostplugin.yaml
    kubectl create -f hostplugin.yaml
}

function create_or_delete_csi_driver_service() {
    local action=$1
    temp_svc=${TEMP_DIR}/service.yaml
    cp "${SNAPSHOT_METADATA_DRIVER_SERVICE}" "${temp_svc}"
    namespace=$NAMESPACE yq -i ".metadata.namespace = env(namespace)" "${temp_svc}"
    yq -i '.spec.selector."app.kubernetes.io/name" = "csi-hostpathplugin"' "${temp_svc}"
    kubectl_retry "${action}" -f "${temp_svc}"
}

function deploy() {
    create_or_delete_crds "create"
    create_or_delete_tls_certs "create"
    create_or_delete_rbacs "create"
    create_or_delete_snapshot_metadata_service "create"
    patch_snapshot_metadata_sidecar
    create_or_delete_csi_driver_service "create"

    kubectl_retry get all -n "${NAMESPACE}"
}

function cleanup() {
    create_or_delete_csi_driver_service "delete"
    create_or_delete_snapshot_metadata_service "delete"
    create_or_delete_rbacs "delete"
    create_or_delete_tls_certs "delete"
    create_or_delete_crds "delete"
}

FUNCTION="$1"
shift # remove function arg now that we've recorded it
# call the function with the remainder of the user-provided args
# -e, -E, and -o=pipefail will ensure this script returns a failure if a part of the function fails
$FUNCTION "$@"