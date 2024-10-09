FROM alpine AS builder
ARG GRPC_HEALTH_PROBE_VERSION=v0.4.34
ADD https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 /bin/grpc_health_probe
RUN chmod +x /bin/grpc_health_probe

FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshot Metadata Sidecar"
ARG binary=./bin/csi-snapshot-metadata
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe
COPY ${binary} csi-snapshot-metadata
ENTRYPOINT ["/csi-snapshot-metadata"]

