#!/bin/bash

create_kind_cluster() {
  # setup kind cluster
  kind create cluster \
    --retain \
    --wait=1m \
    -v=3

  # debug cluster version
  kubectl version
}

install_resources() {
  kubectl apply -k test/test_resources
}

setup() {
  create_kind_cluster

  install_resources
}

run_tests() {
  go test github.com/kubernetes-csi/external-snapshot-metadata/pkg/e2e_test
}

tear_down() {
  ## TODO: check if cluster was created??
  # kind export logs
  kind delete cluster || true
}

# setup signal handlers
# shellcheck disable=SC2317 # this is not unreachable code
signal_handler() {
  if [ -n "${GINKGO_PID:-}" ]; then
    kill -TERM "$GINKGO_PID" || true
  fi
  tear_down
}
trap signal_handler INT TERM

main() {
  # create temp dir and setup cleanup
  TMP_DIR=$(mktemp -d)

  # export the KUBECONFIG to a unique path for testing
  KUBECONFIG="${HOME}/.kube/kind-test-config"
  export KUBECONFIG
  echo "exported KUBECONFIG=${KUBECONFIG}"

  # debug kind version
  kind version

  # create the cluster and run tests
  res=0
  setup || res=$?
  run_tests || res=$?
  tear_down || res=$?
  exit $res
}

main