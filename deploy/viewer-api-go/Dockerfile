FROM golang:1.25.12-bookworm AS build

WORKDIR /workspace/apps/viewer-api-go

COPY apps/viewer-api-go/go.mod apps/viewer-api-go/go.sum ./
RUN go mod download

COPY apps/viewer-api-go ./
RUN CGO_ENABLED=0 go build -o /tmp/viewer-api-go ./cmd/viewer-api \
    && CGO_ENABLED=0 go build -o /tmp/state-doctor ./cmd/state-doctor \
    && CGO_ENABLED=0 go build -o /tmp/state-backup ./cmd/state-backup

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

ENV VIEWER_API_GO_ADDR=:8080 \
    VIEWER_API_DATA_DIR=/data

COPY --from=build /tmp/viewer-api-go /usr/local/bin/viewer-api-go
COPY --from=build /tmp/state-doctor /usr/local/bin/state-doctor
COPY --from=build /tmp/state-backup /usr/local/bin/state-backup

USER 1000:1000

CMD ["viewer-api-go"]
