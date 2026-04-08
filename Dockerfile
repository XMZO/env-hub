FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /env-hub .

FROM scratch
COPY --from=build /env-hub /env-hub
VOLUME /data
EXPOSE 8080
ENV DATA_DIR=/data
ENTRYPOINT ["/env-hub"]
