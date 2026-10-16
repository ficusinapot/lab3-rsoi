FROM arigaio/atlas@sha256:1cba0a901ca5db8bd402c68402607ed47f8eda894d6f865579f3ebaf3b015696 AS atlas
FROM mikefarah/yq:4.47.1 AS yq

FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY modules/platform/go.mod modules/platform/go.sum /src/modules/platform/
COPY services/reservation/go.mod services/reservation/go.sum /src/services/reservation/
WORKDIR /src/services/reservation
RUN go mod download
COPY modules/platform /src/modules/platform
COPY services/reservation /src/services/reservation
RUN CGO_ENABLED=0 go build -o /reservation ./cmd/reservation

FROM alpine:3.23
RUN adduser -D -u 10001 service
WORKDIR /app
COPY --from=build /reservation /usr/local/bin/reservation
COPY --from=atlas --chmod=755 /atlas /usr/local/bin/atlas
COPY --from=yq /usr/bin/yq /usr/local/bin/yq
COPY services/reservation/migrations /app/migrations
COPY --chmod=755 services/reservation/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY services/reservation/config.docker.yaml /app/config.yaml
COPY services/reservation/migrations.yaml /app/migrations.yaml
USER service
EXPOSE 8070
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["--config", "/app/config.yaml"]
