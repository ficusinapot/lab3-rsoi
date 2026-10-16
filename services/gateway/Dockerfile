FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY modules/platform/go.mod modules/platform/go.sum /src/modules/platform/
COPY services/gateway/go.mod services/gateway/go.sum /src/services/gateway/
WORKDIR /src/services/gateway
RUN go mod download
COPY modules/platform /src/modules/platform
COPY services/gateway /src/services/gateway
RUN CGO_ENABLED=0 go build -o /gateway ./cmd/gateway
FROM alpine:3.23
RUN adduser -D -u 10001 service
COPY --from=build /gateway /usr/local/bin/gateway
COPY services/gateway/config.docker.yaml /app/config.yaml
USER service
EXPOSE 8080
ENTRYPOINT ["gateway"]
CMD ["--config", "/app/config.yaml"]
