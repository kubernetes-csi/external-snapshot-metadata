FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG GRPC_HEALTH_PROBE_VERSION=v0.4.37
ARG TARGETARCH
ARG TARGETOS=linux
ARG LDFLAGS
ADD https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-${TARGETARCH} /bin/grpc_health_probe
RUN chmod +x /bin/grpc_health_probe

# Build csi-snapshot-metadata
WORKDIR /workspace
COPY go.mod go.sum ./
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY client/ client/
COPY vendor/ vendor/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=vendor -a -ldflags "${LDFLAGS}" \
    -o /bin/csi-snapshot-metadata \
    ./cmd/csi-snapshot-metadata

FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshot Metadata Sidecar"
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe
COPY --from=builder /bin/csi-snapshot-metadata /csi-snapshot-metadata
ENTRYPOINT ["/csi-snapshot-metadata"]

