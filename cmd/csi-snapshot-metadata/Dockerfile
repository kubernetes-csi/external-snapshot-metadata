FROM gcr.io/distroless/static:latest
LABEL maintainers="Kubernetes Authors"
LABEL description="CSI External Snapshot Metadata Sidecar"
ARG binary=./bin/csi-snapshot-metadata

COPY ${binary} csi-snapshot-metadata
ENTRYPOINT ["/csi-snapshot-metadata"]

