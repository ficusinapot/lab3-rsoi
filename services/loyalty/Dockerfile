FROM arigaio/atlas@sha256:1cba0a901ca5db8bd402c68402607ed47f8eda894d6f865579f3ebaf3b015696 AS atlas
FROM mikefarah/yq:4.47.1 AS yq

FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY modules/platform/go.mod modules/platform/go.sum /src/modules/platform/
COPY services/loyalty/go.mod services/loyalty/go.sum /src/services/loyalty/
WORKDIR /src/services/loyalty
RUN go mod download
COPY modules/platform /src/modules/platform
COPY services/loyalty /src/services/loyalty
RUN CGO_ENABLED=0 go build -o /loyalty ./cmd/loyalty

FROM alpine:3.23
RUN adduser -D -u 10001 service
WORKDIR /app
COPY --from=build /loyalty /usr/local/bin/loyalty
COPY --from=atlas --chmod=755 /atlas /usr/local/bin/atlas
COPY --from=yq /usr/bin/yq /usr/local/bin/yq
COPY services/loyalty/migrations /app/migrations
COPY --chmod=755 services/loyalty/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY services/loyalty/config.docker.yaml /app/config.yaml
COPY services/loyalty/migrations.yaml /app/migrations.yaml
USER service
EXPOSE 8050
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["--config", "/app/config.yaml"]
