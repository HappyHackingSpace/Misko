FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
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
