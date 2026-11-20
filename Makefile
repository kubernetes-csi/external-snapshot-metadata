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

all: build

.PHONY: proto
# Build the Kubernetes SnapshotMetadata gRPC Service Go stubs.
proto:
	protoc -I=proto \
		--go_out=pkg/api --go_opt=paths=source_relative \
		--go-grpc_out=pkg/api --go-grpc_opt=paths=source_relative \
		proto/*.proto

.PHONY: crd
# Generate CRD manifest using controller-gen
crd:
	@ cd client && ./hack/update-crd.sh

.PHONY: codegen
# Generate clientset/informers/listers and deepcopy code
codegen:
	@ cd client && ./hack/update-generated-code.sh

.PHONY: lint
# Run golangci-lint
lint:
	golangci-lint run

# Include release-tools

CMDS=csi-snapshot-metadata

include release-tools/build.make

# Keep mockgen in lockstep with the go.uber.org/mock module in go.mod.
MOCKGEN_VERSION := $(shell go list -m -f '{{.Version}}' go.uber.org/mock)

# Fail CI if gomock clients are stale (interface changed without go generate).
.PHONY: test-mocks
test: test-mocks
test-mocks:
	@ echo; echo "### $@:"
	@ go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)
	@ PATH="$$(go env GOPATH)/bin:$$PATH" go generate ./pkg/csiclientmocks ./pkg/k8sclientmocks
	@ if ! git diff --exit-code -- pkg/csiclientmocks/client.mock.go pkg/k8sclientmocks/client.mock.go; then \
		echo "ERROR: generated mocks are out of date; run:"; \
		echo "  go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)"; \
		echo "  PATH=\"\$$(go env GOPATH)/bin:\$$PATH\" go generate ./pkg/csiclientmocks ./pkg/k8sclientmocks"; \
		exit 1; \
	fi

.PHONY: examples
examples:
	mkdir -p bin
	for d in ./examples/* ; do if [[ -f $$d/main.go ]]; then (cd $$d && go build $(GOFLAGS_VENDOR) -a -ldflags '$(FULL_LDFLAGS)' -o "$(abspath ./bin)/" .); fi; done

# Eventually extend the test target to include lint.
# Currently the linter is not available in the CI infrastructure.
#test: lint
