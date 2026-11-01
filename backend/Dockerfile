FROM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bootstrap ./cmd/bootstrap && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/jobs ./cmd/jobs && \
    mkdir -p /out/data/videos

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/api /api
COPY --from=build /out/bootstrap /bootstrap
COPY --from=build /out/jobs /jobs
# LOCAL_STORAGE_DIR mount point: a new named volume copies this ownership.
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 4000
ENTRYPOINT ["/api"]
