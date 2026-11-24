FROM alpine:3.24 AS assets
# hadolint ignore=DL3018
RUN apk add --no-cache bash curl tar coreutils
WORKDIR /src
COPY scripts/fetch-assets.sh scripts/fetch-assets.sh
RUN mkdir -p web/static && bash scripts/fetch-assets.sh

FROM golang:1.26.11-alpine3.24 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY web web
COPY --from=assets /src/web/static/vendor web/static/vendor
RUN test -f web/static/vendor/ffmpeg/ffmpeg-core.wasm \
 && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /mkvcheck ./cmd/mkvcheck \
 && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /mkvcheck /mkvcheck
COPY --from=builder --chown=65532:65532 /data /data
USER 65532:65532
VOLUME /data
ENV MKVCHECK_DATA_DIR=/data MKVCHECK_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/mkvcheck","healthcheck"]
ENTRYPOINT ["/mkvcheck"]
