# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22

RUN adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=build /out/server /usr/local/bin/remitly-stock-market

USER appuser
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/remitly-stock-market"]
