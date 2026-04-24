# API TribIA (Railway, Fly.io, etc.) — build multi-estágio, binário estático
# (Go 1.26+ via GOTOOLCHAIN conforme go.mod)
# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
ENV CGO_ENABLED=0
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/tribia-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
  && adduser -D -H -u 65532 -g app app
COPY --from=build /out/tribia-api /usr/local/bin/tribia-api
USER app
EXPOSE 8080
ENV PORT=8080
# Railway injeta PORT; a app também aceita 8080 por omissão (config.Port).
CMD ["/usr/local/bin/tribia-api"]
