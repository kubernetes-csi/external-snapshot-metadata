# Release notes for v0.1.0

[Documentation](https://kubernetes-csi.github.io)

## Changes

- Add protobuf definitions for SnapshotMetadata Service, Makefile and make target to compile and generate gRPC code. ([#1](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/1) ,[@PrasadG193](https://github.com/PrasadG193)).
- Add new SnapshotMetadataService CRD ([#2](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/2) ,[@PrasadG193](https://github.com/PrasadG193)).
- Add grpc server for SnapshotMetadata service ([#4](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/4) ,[@PrasadG193](https://github.com/PrasadG193)).
- Add release-tools files and makefile reference ([#6](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/6) ,[@hairyhum](https://github.com/hairyhum)).
- Add authentication and authorization layer ([#7](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/7) ,[@PrasadG193](https://github.com/PrasadG193)).
- Added the gRPC health service ([#20](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/20) ,[@carlbraganza](https://github.com/carlbraganza)).
- Add deployment manifests to setup sidecar server ([#26](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/26) ,[@PrasadG193](https://github.com/PrasadG193)).
- Implement sidecar to CSI driver communication ([#27](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/27) ,[@PrasadG193](https://github.com/PrasadG193)).
- Fetch the optional snapshotter secrets from the VolumeSnapshotClass ([#34](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/34) ,[@carlbraganza](https://github.com/carlbraganza)).
- Use a contextual logger in the sidecar handlers ([#37](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/37),[@carlbraganza](https://github.com/carlbraganza)).
- Verify base and target snapshot belong to the same volume ([#38](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/38) ,[@PrasadG193](https://github.com/PrasadG193)).
- Log stream responses with progress ([#49](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/49),[@carlbraganza](https://github.com/carlbraganza)).
- Example illustrating how an application obtains snapshot metadata ([#64](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/64),[@carlbraganza](https://github.com/carlbraganza)).
- Add integration test with github action ([#68](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/68),[@Rakshith-R](https://github.com/Rakshith-R)).
- Add prometheus metrics for the sidecar ([#78](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/78),[@Nikhil-Ladha](https://github.com/Nikhil-Ladha)).
- Add tls secrets and CR for e2e test setup ([#84](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/84) ,[@PrasadG193](https://github.com/PrasadG193)).
- Add snapshot-metadata-lister pod deployment ([#93](https://github.com/kubernetes-csi/external-snapshot-metadata/pull/93) ,[@PrasadG193](https://github.com/PrasadG193)).

## Dependencies

### Added
- github.com/container-storage-interface/spec v1.11.0
- github.com/golang/mock v1.6.0
- github.com/kubernetes-csi/csi-lib-utils v0.20.0
- github.com/kubernetes-csi/csi-test/v5 v5.3.1
- github.com/kubernetes-csi/external-snapshot-metadata/client v0.0.0-20240708191355-ca55d80f214a
- github.com/kubernetes-csi/external-snapshotter/client/v8 v8.2.0
- github.com/kubernetes-csi/external-snapshotter/v8 v8.2.0
- github.com/stretchr/testify v1.10.0
- google.golang.org/grpc v1.69.4
- google.golang.org/protobuf v1.36.3
- k8s.io/api v0.32.1
- k8s.io/apimachinery v0.32.1
- k8s.io/apiserver v0.32.1
- k8s.io/client-go v0.32.1
- k8s.io/klog/v2 v2.130.1
- k8s.io/apimachinery v0.30.1
- k8s.io/client-go v0.30.1
- k8s.io/code-generator v0.30.1
- github.com/davecgh/go-spew v1.1.1
- github.com/emicklei/go-restful/v3 v3.11.0
- github.com/evanphx/json-patch v4.12.0+incompatible
- github.com/go-logr/logr v1.4.1
- github.com/go-openapi/jsonpointer v0.19.6
- github.com/go-openapi/jsonreference v0.20.2
- github.com/go-openapi/swag v0.22.3
- github.com/google/gnostic-models v0.6.8
- github.com/google/uuid v1.3.0
- github.com/mailru/easyjson v0.7.7
- golang.org/x/mod v0.15.0
- golang.org/x/net v0.23.0
- golang.org/x/oauth2 v0.10.0
- golang.org/x/sys v0.18.0
- golang.org/x/term v0.18.0
- golang.org/x/text v0.14.0
- golang.org/x/time v0.3.0
- golang.org/x/tools v0.18.0
- google.golang.org/appengine v1.6.7
- google.golang.org/protobuf v1.33.0
- gopkg.in/yaml.v2 v2.4.0
- k8s.io/api v0.30.1
- k8s.io/gengo/v2 v2.0.0-20240228010128-51d4e06bde70
- k8s.io/klog/v2 v2.120.1
- k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340
- k8s.io/utils v0.0.0-20230726121419-3b25d923346b
- sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd
- sigs.k8s.io/structured-merge-diff/v4 v4.4.1
- sigs.k8s.io/yaml v1.3.0
- github.com/beorn7/perks v1.0.1
- github.com/blang/semver/v4 v4.0.0
- github.com/cespare/xxhash/v2 v2.3.0
- github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
- github.com/emicklei/go-restful/v3 v3.12.1
- github.com/fxamacker/cbor/v2 v2.7.0
- github.com/go-logr/logr v1.4.2
- github.com/go-logr/stdr v1.2.2
- github.com/go-openapi/jsonpointer v0.21.0
- github.com/go-openapi/jsonreference v0.21.0
- github.com/go-openapi/swag v0.23.0
- github.com/gogo/protobuf v1.3.2
- github.com/golang/protobuf v1.5.4
- github.com/google/gnostic-models v0.6.9
- github.com/google/go-cmp v0.6.0
- github.com/google/gofuzz v1.2.0
- github.com/google/uuid v1.6.0
- github.com/josharian/intern v1.0.0
- github.com/json-iterator/go v1.1.12
- github.com/klauspost/compress v1.17.11
- github.com/mailru/easyjson v0.9.0
- github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
- github.com/modern-go/reflect2 v1.0.2
- github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
- github.com/pkg/errors v0.9.1
- github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2
- github.com/prometheus/client_golang v1.20.5
- github.com/prometheus/client_model v0.6.1
- github.com/prometheus/common v0.62.0
- github.com/prometheus/procfs v0.15.1
- github.com/spf13/pflag v1.0.5
- github.com/x448/float16 v0.8.4
- go.opentelemetry.io/auto/sdk v1.1.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.59.0
- go.opentelemetry.io/otel v1.34.0
- go.opentelemetry.io/otel/metric v1.34.0
- go.opentelemetry.io/otel/trace v1.34.0
- golang.org/x/net v0.34.0
- golang.org/x/oauth2 v0.25.0
- golang.org/x/sys v0.29.0
- golang.org/x/term v0.28.0
- golang.org/x/text v0.21.0
- golang.org/x/time v0.9.0
- google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f
- gopkg.in/evanphx/json-patch.v4 v4.12.0
- gopkg.in/inf.v0 v0.9.1
- gopkg.in/yaml.v3 v3.0.1
- k8s.io/component-base v0.32.1
- k8s.io/kube-openapi v0.0.0-20241212222426-2c72e554b1e7
- k8s.io/utils v0.0.0-20241210054802-24370beab758
- sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8
- sigs.k8s.io/structured-merge-diff/v4 v4.5.0
- sigs.k8s.io/yaml v1.4.0