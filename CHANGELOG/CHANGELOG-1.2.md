# Release notes for v1.2.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v1.1.0

## Changes by Kind

### Feature

- Add --tls-min-version, --tls-cipher-suites, and --tls-curve-preferences flags to configure TLS parameters for the gRPC server. ([#259](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/259), [@Rakshith-R](https://github.com/Rakshith-R))
- Add `spec.requireBaseSnapshotForDelta` optional boolean field to SnapshotMetadataService CRD. CSI drivers can set this to `true` to indicate that the base volume snapshot must be retained for computing CBT deltas, or `false` to indicate base snapshots can be safely deleted after obtaining CBT metadata. When unset, no guidance is provided and backup clients should act at their discretion. ([#258](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/258), [@Rakshith-R](https://github.com/Rakshith-R))

### Bug or Regression

- The snapshot metadata iterator now returns an error instead of panicking when a previous VolumeSnapshot or its VolumeSnapshotContent does not yet have status. ([#287](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/287), [@git-jxj](https://github.com/git-jxj))
- `GetMetadataAllocated` and `GetMetadataDelta` now return `NotFound` instead of `Unavailable` when the requested VolumeSnapshot or its VolumeSnapshotContent does not exist. Clients that retried on `Unavailable` for this case should treat `NotFound` as non-retryable. ([#295](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/295), [@iPraveenParihar](https://github.com/iPraveenParihar))
- Fix --audience flag being silently ignored due to Go struct field shadowing. The sidecar now correctly reads the audience from the command-line flag. ([#264](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/264), [@Rakshith-R](https://github.com/Rakshith-R))

## Dependencies

### Added
- github.com/Masterminds/semver/v3: [v3.4.0](https://github.com/Masterminds/semver/tree/v3.4.0)
- github.com/go-openapi/swag/pools: [v0.29.2](https://github.com/go-openapi/swag/tree/pools/v0.29.2)
- github.com/pandatix/go-abnf: [v0.5.5](https://github.com/pandatix/go-abnf/tree/v0.5.5)
- go.opentelemetry.io/otel/log: v1.47.0
- go.uber.org/mock: v0.6.0

### Changed
- cel.dev/expr: v0.25.2 → v0.25.3
- github.com/Azure/go-ansiterm: [306776e → faa5f7b](https://github.com/Azure/go-ansiterm/compare/306776e...faa5f7b)
- github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp: [v1.32.0 → v1.35.0](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/compare/detectors/gcp/v1.32.0...detectors/gcp/v1.35.0)
- github.com/container-storage-interface/spec: [v1.12.0 → v1.13.0](https://github.com/container-storage-interface/spec/compare/v1.12.0...v1.13.0)
- github.com/emicklei/go-restful/v3: [v3.13.0 → v3.14.0](https://github.com/emicklei/go-restful/compare/v3.13.0...v3.14.0)
- github.com/envoyproxy/go-control-plane/envoy: [v1.37.0 → e6e3fd9](https://github.com/envoyproxy/go-control-plane/compare/envoy/v1.37.0...e6e3fd9)
- github.com/felixge/httpsnoop: [v1.0.4 → v1.1.0](https://github.com/felixge/httpsnoop/compare/v1.0.4...v1.1.0)
- github.com/fxamacker/cbor/v2: [v2.9.2 → v2.10.0](https://github.com/fxamacker/cbor/compare/v2.9.2...v2.10.0)
- github.com/go-logr/logr: [v1.4.3 → v1.4.4](https://github.com/go-logr/logr/compare/v1.4.3...v1.4.4)
- github.com/go-openapi/jsonpointer: [v0.24.0 → v1.0.2](https://github.com/go-openapi/jsonpointer/compare/v0.24.0...v1.0.2)
- github.com/go-openapi/jsonreference: [v0.21.6 → v1.0.3](https://github.com/go-openapi/jsonreference/compare/v0.21.6...v1.0.3)
- github.com/go-openapi/swag/cmdutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/cmdutils/v0.27.0...cmdutils/v0.29.2)
- github.com/go-openapi/swag/conv: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/conv/v0.27.0...conv/v0.29.2)
- github.com/go-openapi/swag/fileutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/fileutils/v0.27.0...fileutils/v0.29.2)
- github.com/go-openapi/swag/jsonutils/fixtures_test: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/jsonutils/fixtures_test/v0.27.0...jsonutils/fixtures_test/v0.29.2)
- github.com/go-openapi/swag/jsonutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/jsonutils/v0.27.0...jsonutils/v0.29.2)
- github.com/go-openapi/swag/loading: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/loading/v0.27.0...loading/v0.29.2)
- github.com/go-openapi/swag/mangling: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/mangling/v0.27.0...mangling/v0.29.2)
- github.com/go-openapi/swag/netutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/netutils/v0.27.0...netutils/v0.29.2)
- github.com/go-openapi/swag/stringutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/stringutils/v0.27.0...stringutils/v0.29.2)
- github.com/go-openapi/swag/typeutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/typeutils/v0.27.0...typeutils/v0.29.2)
- github.com/go-openapi/swag/yamlutils: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/yamlutils/v0.27.0...yamlutils/v0.29.2)
- github.com/go-openapi/swag: [v0.27.0 → v0.29.2](https://github.com/go-openapi/swag/compare/v0.27.0...v0.29.2)
- github.com/go-openapi/testify/enable/yaml/v2: [v2.6.0 → v2.7.0](https://github.com/go-openapi/testify/compare/enable/yaml/v2/v2.6.0...enable/yaml/v2/v2.7.0)
- github.com/go-openapi/testify/v2: [v2.6.0 → v2.8.0](https://github.com/go-openapi/testify/compare/v2.6.0...v2.8.0)
- github.com/google/cel-go: [v0.28.1 → v0.29.2](https://github.com/google/cel-go/compare/v0.28.1...v0.29.2)
- github.com/google/gofuzz: [v1.2.0 → v1.0.0](https://github.com/google/gofuzz/compare/v1.2.0...v1.0.0)
- github.com/google/pprof: [40e02aa → 545e8a4](https://github.com/google/pprof/compare/40e02aa...545e8a4)
- github.com/grpc-ecosystem/go-grpc-middleware/v2: [v2.3.3 → v2.3.4](https://github.com/grpc-ecosystem/go-grpc-middleware/compare/v2.3.3...v2.3.4)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.29.0 → v2.31.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.29.0...v2.31.0)
- github.com/klauspost/compress: [v1.18.0 → v1.20.1](https://github.com/klauspost/compress/compare/v1.18.0...v1.20.1)
- github.com/kubernetes-csi/csi-lib-utils: [v0.24.0 → v0.25.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.24.0...v0.25.0)
- github.com/kubernetes-csi/csi-test/v5: [v5.4.0 → v5.6.0](https://github.com/kubernetes-csi/csi-test/compare/v5.4.0...v5.6.0)
- github.com/kubernetes-csi/external-snapshotter/client/v8: [v8.6.0 → v8.7.0](https://github.com/kubernetes-csi/external-snapshotter/compare/client/v8/v8.6.0...client/v8/v8.7.0)
- github.com/kubernetes-csi/external-snapshotter/v8: [v8.6.0 → v8.7.0](https://github.com/kubernetes-csi/external-snapshotter/compare/v8.6.0...v8.7.0)
- github.com/moby/term: [v0.5.0 → v0.5.2](https://github.com/moby/term/compare/v0.5.0...v0.5.2)
- github.com/onsi/ginkgo/v2: [v2.22.0 → v2.32.1](https://github.com/onsi/ginkgo/compare/v2.22.0...v2.32.1)
- github.com/onsi/gomega: [v1.36.1 → v1.42.1](https://github.com/onsi/gomega/compare/v1.36.1...v1.42.1)
- github.com/prometheus/client_golang: [v1.23.2 → v1.25.0](https://github.com/prometheus/client_golang/compare/v1.23.2...v1.25.0)
- github.com/prometheus/client_model: [v0.6.2 → v0.6.3](https://github.com/prometheus/client_model/compare/v0.6.2...v0.6.3)
- github.com/prometheus/common: [v0.69.0 → v0.74.0](https://github.com/prometheus/common/compare/v0.69.0...v0.74.0)
- github.com/prometheus/procfs: [v0.21.1 → v0.23.0](https://github.com/prometheus/procfs/compare/v0.21.1...v0.23.0)
- github.com/sirupsen/logrus: [v1.9.3 → v1.9.4](https://github.com/sirupsen/logrus/compare/v1.9.3...v1.9.4)
- github.com/spf13/pflag: [v1.0.10 → v1.1.0](https://github.com/spf13/pflag/compare/v1.0.10...v1.1.0)
- github.com/spiffe/go-spiffe/v2: [v2.6.0 → v2.8.1](https://github.com/spiffe/go-spiffe/compare/v2.6.0...v2.8.1)
- github.com/stretchr/objx: [v0.5.2 → v0.5.3](https://github.com/stretchr/objx/compare/v0.5.2...v0.5.3)
- github.com/stretchr/testify: [v1.11.1 → v1.12.1](https://github.com/stretchr/testify/compare/v1.11.1...v1.12.1)
- github.com/yuin/goldmark: [v1.3.5 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.3.5...v1.4.13)
- go.etcd.io/bbolt: v1.4.3 → v1.5.0
- go.etcd.io/etcd/api/v3: v3.6.11 → v3.7.2
- go.etcd.io/etcd/client/pkg/v3: v3.6.11 → v3.7.2
- go.etcd.io/etcd/client/v3: v3.6.11 → v3.7.2
- go.etcd.io/etcd/pkg/v3: v3.6.8 → v3.7.0
- go.etcd.io/etcd/server/v3: v3.6.8 → v3.7.0
- go.etcd.io/raft/v3: v3.6.0 → v3.7.0
- go.opentelemetry.io/contrib/detectors/gcp: v1.43.0 → v1.45.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.69.0 → v0.72.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.68.0 → v0.71.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/metric: v1.44.0 → v1.47.0
- go.opentelemetry.io/otel/sdk/metric: v1.44.0 → v1.47.0
- go.opentelemetry.io/otel/sdk: v1.44.0 → v1.47.0
- go.opentelemetry.io/otel/trace: v1.44.0 → v1.47.0
- go.opentelemetry.io/otel: v1.44.0 → v1.47.0
- go.opentelemetry.io/proto/otlp: v1.10.0 → v1.11.0
- go.yaml.in/yaml/v2: v2.4.4 → v2.4.5
- go.yaml.in/yaml/v3: v3.0.4 → v3.0.6
- golang.org/x/crypto: v0.53.0 → v0.58.0
- golang.org/x/exp: 716be56 → 85c1c22
- golang.org/x/mod: v0.37.0 → v0.41.0
- golang.org/x/net: v0.56.0 → v0.62.0
- golang.org/x/oauth2: v0.36.0 → v0.38.0
- golang.org/x/sync: v0.21.0 → v0.24.0
- golang.org/x/sys: v0.46.0 → v0.49.0
- golang.org/x/term: v0.44.0 → v0.47.0
- golang.org/x/text: v0.39.0 → v0.43.0
- golang.org/x/time: v0.15.0 → v0.16.0
- golang.org/x/tools: v0.47.0 → v0.51.0
- google.golang.org/genproto/googleapis/api: afd174a → 8a89bd6
- google.golang.org/genproto/googleapis/rpc: 3dc84a4 → 8a89bd6
- google.golang.org/grpc: v1.82.0 → v1.85.0
- google.golang.org/protobuf: f2248ac → v1.36.12
- k8s.io/api: v0.36.1 → v0.37.2
- k8s.io/apiextensions-apiserver: v0.36.1 → v0.37.2
- k8s.io/apimachinery: v0.36.1 → v0.37.2
- k8s.io/apiserver: v0.36.1 → v0.37.2
- k8s.io/client-go: v0.36.1 → v0.37.2
- k8s.io/code-generator: v0.36.1 → v0.37.2
- k8s.io/component-base: v0.36.1 → v0.37.2
- k8s.io/component-helpers: v0.36.1 → v0.37.2
- k8s.io/gengo/v2: ec3ebc5 → 25e2208
- k8s.io/kms: v0.36.1 → v0.37.2
- k8s.io/kube-openapi: 43fb72c → be32def
- k8s.io/streaming: v0.36.1 → v0.37.2
- k8s.io/utils: b8788ab → cf1189d
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.35.0 → v0.36.0
- sigs.k8s.io/structured-merge-diff/v6: v6.4.0 → v6.4.2

### Removed
- github.com/go-openapi/swag/jsonname: [v0.26.0](https://github.com/go-openapi/swag/tree/jsonname/v0.26.0)
- github.com/gogo/protobuf: [v1.3.2](https://github.com/gogo/protobuf/tree/v1.3.2)
- github.com/golang/mock: [v1.6.0](https://github.com/golang/mock/tree/v1.6.0)
- github.com/josharian/intern: [v1.0.0](https://github.com/josharian/intern/tree/v1.0.0)
- github.com/mailru/easyjson: [v0.9.0](https://github.com/mailru/easyjson/tree/v0.9.0)
- github.com/pkg/errors: [v0.9.1](https://github.com/pkg/errors/tree/v0.9.1)
- github.com/stoewer/go-strcase: [v1.3.0](https://github.com/stoewer/go-strcase/tree/v1.3.0)
- golang.org/x/xerrors: 5ec99f8
- google.golang.org/appengine: v1.6.7
- sigs.k8s.io/structured-merge-diff/v4: v4.4.1
