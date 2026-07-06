FROM golang:1.25.11-bookworm AS build

WORKDIR /workspace/services/novel-fetcher

COPY services/novel-fetcher/go.mod services/novel-fetcher/go.sum ./
RUN go mod download

COPY services/novel-fetcher ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/novel-fetcher ./cmd/novel-fetcher

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/novel-fetcher /novel-fetcher

USER 1000:1000
EXPOSE 33006
ENTRYPOINT ["/novel-fetcher"]
