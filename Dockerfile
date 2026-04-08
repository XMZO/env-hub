FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /env-hub .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /env-hub /usr/local/bin/env-hub
VOLUME /data
EXPOSE 8080
ENV DATA_DIR=/data
ENTRYPOINT ["env-hub"]
