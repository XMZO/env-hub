FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /env-hub .

FROM alpine:3.22
RUN apk add --no-cache bash ca-certificates curl docker-cli
COPY --from=build /env-hub /env-hub
VOLUME /data
EXPOSE 9800
ENV DATA_DIR=/data
ENV DOCKER_HOST=unix:///var/run/docker.sock
ENTRYPOINT ["/env-hub"]
