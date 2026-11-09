# Static Go binary on a small Alpine base, built for linux/arm64 (the
# Raspberry Pi) and linux/amd64 by CI.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY templates ./templates
COPY static ./static
ARG VERSION=docker
RUN go vet ./... && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -trimpath -o /out/vinyl-catalogue .

FROM alpine:3.21
# ca-certificates: TLS to api.discogs.com and i.discogs.com; tzdata: TZ= for dates and logs.
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/vinyl-catalogue /usr/local/bin/vinyl-catalogue

# Owned by 1000:1000 so a fresh named volume inherits a writable /data.
RUN mkdir -p /data && chown 1000:1000 /data
USER 1000:1000
ENV DATA_DIR=/data LISTEN_ADDR=:8097
VOLUME ["/data"]
EXPOSE 8097

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8097/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/vinyl-catalogue"]
