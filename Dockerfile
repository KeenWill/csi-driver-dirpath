# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.22.2-alpine3.19@sha256:cdc86d9f363e8786845bea2040312b4efa321b828acdeb26f393faa864d887b0 AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /dirpath-plugin ./cmd/dirpath-plugin

FROM scratch
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/KeenWill/csi-driver-dirpath" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
COPY --from=build /dirpath-plugin /dirpath-plugin
ENTRYPOINT ["/dirpath-plugin"]
