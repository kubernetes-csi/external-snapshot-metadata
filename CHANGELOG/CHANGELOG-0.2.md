# Release notes for v0.2.0

[Documentation](https://kubernetes-csi.github.io)

## Urgent Upgrade Notes

### (No, really, you MUST read this before you upgrade)

- Action required
  The signature of the Kubernetes GetMetadataRequest RPC call has changed. ([#180](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/180), [@carlbraganza](https://github.com/carlbraganza))

## Changes by Kind

### Feature

- Add support for passing audience token through cmdline arguments ([#162](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/162), [@Rakshith-R](https://github.com/Rakshith-R))
- Add verify functionality to snapshot-metadata-lister tool ([#117](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/117), [@Rakshith-R](https://github.com/Rakshith-R))

### Other (Cleanup or Flake)

- Update kubernetes dependencies to v1.34.0 ([#177](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/177), [@dobsonj](https://github.com/dobsonj))

## Dependencies

### Added
- github.com/envoyproxy/go-control-plane/envoy: [v1.32.4](https://github.com/envoyproxy/go-control-plane/tree/envoy/v1.32.4)
- github.com/envoyproxy/go-control-plane/ratelimit: [v0.1.0](https://github.com/envoyproxy/go-control-plane/tree/ratelimit/v0.1.0)
- github.com/go-jose/go-jose/v4: [v4.0.4](https://github.com/go-jose/go-jose/tree/v4.0.4)
- github.com/golang-jwt/jwt/v5: [v5.2.2](https://github.com/golang-jwt/jwt/tree/v5.2.2)
- github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus: [v1.0.1](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/providers/prometheus/v1.0.1)
- github.com/grpc-ecosystem/go-grpc-middleware/v2: [v2.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v2.3.0)
- github.com/spiffe/go-spiffe/v2: [v2.5.0](https://github.com/spiffe/go-spiffe/tree/v2.5.0)
- github.com/zeebo/errs: [v1.4.0](https://github.com/zeebo/errs/tree/v1.4.0)
- go.etcd.io/raft/v3: v3.6.0
- go.uber.org/automaxprocs: v1.6.0
- go.yaml.in/yaml/v2: v2.4.2
- go.yaml.in/yaml/v3: v3.0.4
- gopkg.in/go-jose/go-jose.v2: v2.6.3
- k8s.io/apiextensions-apiserver: v0.34.0
- sigs.k8s.io/randfill: v1.0.0
- sigs.k8s.io/structured-merge-diff/v6: v6.3.0

### Changed
- cel.dev/expr: v0.18.0 → v0.24.0
- cloud.google.com/go/compute/metadata: v0.5.2 → v0.6.0
- github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp: [v1.24.2 → v1.26.0](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/compare/detectors/gcp/v1.24.2...detectors/gcp/v1.26.0)
- github.com/cncf/xds/go: [b4127c9 → 2f00578](https://github.com/cncf/xds/compare/b4127c9...2f00578)
- github.com/container-storage-interface/spec: [v1.11.0 → v1.12.0](https://github.com/container-storage-interface/spec/compare/v1.11.0...v1.12.0)
- github.com/coreos/go-oidc: [v2.2.1+incompatible → v2.3.0+incompatible](https://github.com/coreos/go-oidc/compare/v2.2.1...v2.3.0)
- github.com/emicklei/go-restful/v3: [v3.12.1 → v3.12.2](https://github.com/emicklei/go-restful/compare/v3.12.1...v3.12.2)
- github.com/envoyproxy/go-control-plane: [v0.13.1 → v0.13.4](https://github.com/envoyproxy/go-control-plane/compare/v0.13.1...v0.13.4)
- github.com/envoyproxy/protoc-gen-validate: [v1.1.0 → v1.2.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v1.1.0...v1.2.1)
- github.com/fsnotify/fsnotify: [v1.7.0 → v1.9.0](https://github.com/fsnotify/fsnotify/compare/v1.7.0...v1.9.0)
- github.com/fxamacker/cbor/v2: [v2.7.0 → v2.9.0](https://github.com/fxamacker/cbor/compare/v2.7.0...v2.9.0)
- github.com/go-openapi/jsonpointer: [v0.21.0 → v0.21.1](https://github.com/go-openapi/jsonpointer/compare/v0.21.0...v0.21.1)
- github.com/go-openapi/swag: [v0.23.0 → v0.23.1](https://github.com/go-openapi/swag/compare/v0.23.0...v0.23.1)
- github.com/golang/glog: [v1.2.2 → v1.2.4](https://github.com/golang/glog/compare/v1.2.2...v1.2.4)
- github.com/google/btree: [v1.0.1 → v1.1.3](https://github.com/google/btree/compare/v1.0.1...v1.1.3)
- github.com/google/cel-go: [v0.22.0 → v0.26.0](https://github.com/google/cel-go/compare/v0.22.0...v0.26.0)
- github.com/google/gnostic-models: [v0.6.9 → v0.7.0](https://github.com/google/gnostic-models/compare/v0.6.9...v0.7.0)
- github.com/google/go-cmp: [v0.6.0 → v0.7.0](https://github.com/google/go-cmp/compare/v0.6.0...v0.7.0)
- github.com/google/pprof: [d1b30fe → 40e02aa](https://github.com/google/pprof/compare/d1b30fe...40e02aa)
- github.com/gorilla/websocket: [v1.5.0 → e064f32](https://github.com/gorilla/websocket/compare/v1.5.0...e064f32)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.20.0 → v2.26.3](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.20.0...v2.26.3)
- github.com/jonboulle/clockwork: [v0.4.0 → v0.5.0](https://github.com/jonboulle/clockwork/compare/v0.4.0...v0.5.0)
- github.com/klauspost/compress: [v1.17.11 → v1.18.0](https://github.com/klauspost/compress/compare/v1.17.11...v1.18.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.20.0 → v0.22.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.20.0...v0.22.0)
- github.com/kubernetes-csi/csi-test/v5: [v5.3.1 → v5.4.0](https://github.com/kubernetes-csi/csi-test/compare/v5.3.1...v5.4.0)
- github.com/kubernetes-csi/external-snapshotter/client/v8: [v8.2.0 → v8.4.0](https://github.com/kubernetes-csi/external-snapshotter/compare/client/v8/v8.2.0...client/v8/v8.4.0)
- github.com/kubernetes-csi/external-snapshotter/v8: [v8.2.0 → v8.4.0](https://github.com/kubernetes-csi/external-snapshotter/compare/v8.2.0...v8.4.0)
- github.com/mailru/easyjson: [v0.9.0 → v0.9.1](https://github.com/mailru/easyjson/compare/v0.9.0...v0.9.1)
- github.com/modern-go/reflect2: [v1.0.2 → 35a7c28](https://github.com/modern-go/reflect2/compare/v1.0.2...35a7c28)
- github.com/onsi/ginkgo/v2: [v2.21.0 → v2.22.0](https://github.com/onsi/ginkgo/compare/v2.21.0...v2.22.0)
- github.com/onsi/gomega: [v1.35.1 → v1.36.1](https://github.com/onsi/gomega/compare/v1.35.1...v1.36.1)
- github.com/prometheus/client_golang: [v1.20.5 → v1.22.0](https://github.com/prometheus/client_golang/compare/v1.20.5...v1.22.0)
- github.com/prometheus/client_model: [v0.6.1 → v0.6.2](https://github.com/prometheus/client_model/compare/v0.6.1...v0.6.2)
- github.com/prometheus/common: [v0.62.0 → v0.63.0](https://github.com/prometheus/common/compare/v0.62.0...v0.63.0)
- github.com/prometheus/procfs: [v0.15.1 → v0.16.1](https://github.com/prometheus/procfs/compare/v0.15.1...v0.16.1)
- github.com/spf13/cobra: [v1.8.1 → v1.9.1](https://github.com/spf13/cobra/compare/v1.8.1...v1.9.1)
- github.com/spf13/pflag: [v1.0.5 → v1.0.6](https://github.com/spf13/pflag/compare/v1.0.5...v1.0.6)
- go.etcd.io/bbolt: v1.3.11 → v1.4.2
- go.etcd.io/etcd/api/v3: v3.5.16 → v3.6.4
- go.etcd.io/etcd/client/pkg/v3: v3.5.16 → v3.6.4
- go.etcd.io/etcd/client/v3: v3.5.16 → v3.6.4
- go.etcd.io/etcd/pkg/v3: v3.5.16 → v3.6.4
- go.etcd.io/etcd/server/v3: v3.5.16 → v3.6.4
- go.opentelemetry.io/contrib/detectors/gcp: v1.31.0 → v1.34.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.59.0 → v0.60.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.53.0 → v0.58.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.27.0 → v1.34.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.28.0 → v1.34.0
- go.opentelemetry.io/otel/metric: v1.34.0 → v1.35.0
- go.opentelemetry.io/otel/sdk/metric: v1.31.0 → v1.34.0
- go.opentelemetry.io/otel/sdk: v1.31.0 → v1.34.0
- go.opentelemetry.io/otel/trace: v1.34.0 → v1.35.0
- go.opentelemetry.io/otel: v1.34.0 → v1.35.0
- go.opentelemetry.io/proto/otlp: v1.3.1 → v1.5.0
- golang.org/x/crypto: v0.32.0 → v0.37.0
- golang.org/x/net: v0.34.0 → v0.39.0
- golang.org/x/oauth2: v0.25.0 → v0.30.0
- golang.org/x/sync: v0.10.0 → v0.13.0
- golang.org/x/sys: v0.29.0 → v0.33.0
- golang.org/x/term: v0.28.0 → v0.31.0
- golang.org/x/text: v0.21.0 → v0.24.0
- golang.org/x/time: v0.9.0 → v0.11.0
- golang.org/x/tools: v0.26.0 → v0.28.0
- google.golang.org/genproto/googleapis/api: 796eee8 → a0af3ef
- google.golang.org/genproto/googleapis/rpc: 1a7da9e → a0af3ef
- google.golang.org/grpc: v1.69.4 → v1.72.1
- google.golang.org/protobuf: v1.36.3 → v1.36.6
- k8s.io/api: v0.32.1 → v0.34.0
- k8s.io/apimachinery: v0.32.1 → v0.34.0
- k8s.io/apiserver: v0.32.1 → v0.34.0
- k8s.io/client-go: v0.32.1 → v0.34.0
- k8s.io/code-generator: v0.30.1 → v0.34.0
- k8s.io/component-base: v0.32.1 → v0.34.0
- k8s.io/component-helpers: v0.31.0 → v0.34.0
- k8s.io/gengo/v2: a7b603a → 85fd79d
- k8s.io/kms: v0.32.1 → v0.34.0
- k8s.io/kube-openapi: 2c72e55 → f3f2b99
- k8s.io/utils: 24370be → 4c0f3b2
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.31.0 → v0.31.2
- sigs.k8s.io/structured-merge-diff/v4: v4.5.0 → v4.6.0
- sigs.k8s.io/yaml: v1.4.0 → v1.6.0

### Removed
- github.com/asaskevich/govalidator: [f61b66f](https://github.com/asaskevich/govalidator/tree/f61b66f)
- github.com/census-instrumentation/opencensus-proto: [v0.4.1](https://github.com/census-instrumentation/opencensus-proto/tree/v0.4.1)
- github.com/go-task/slim-sprig: [52ccab3](https://github.com/go-task/slim-sprig/tree/52ccab3)
- github.com/golang-jwt/jwt/v4: [v4.5.0](https://github.com/golang-jwt/jwt/tree/v4.5.0)
- github.com/golang/groupcache: [41bb18b](https://github.com/golang/groupcache/tree/41bb18b)
- github.com/grpc-ecosystem/go-grpc-middleware: [v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v1.3.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.16.0)
- github.com/imdario/mergo: [v0.3.13](https://github.com/imdario/mergo/tree/v0.3.13)
- github.com/kubernetes-csi/external-snapshot-metadata/client: [ca55d80](https://github.com/kubernetes-csi/external-snapshot-metadata/tree/ca55d80)
- go.etcd.io/etcd/client/v2: v2.305.16
- go.etcd.io/etcd/raft/v3: v3.5.16
- google.golang.org/genproto: ef43131
- gopkg.in/square/go-jose.v2: v2.6.0
