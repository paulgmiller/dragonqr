FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dragonqr ./cmd/dragonqr

FROM alpine:3.21

RUN addgroup -S dragonqr && adduser -S -G dragonqr -u 10001 dragonqr

WORKDIR /app

COPY --from=build /out/dragonqr /app/dragonqr
COPY quest.yaml /app/quest.yaml
COPY templates /app/templates
COPY static /app/static

RUN mkdir -p /app/data && chown -R dragonqr:dragonqr /app

USER dragonqr

EXPOSE 8080

ENTRYPOINT ["/app/dragonqr"]
CMD ["-addr", "0.0.0.0:8080", "-quest", "/app/quest.yaml", "-data", "/app/data/players.json"]

