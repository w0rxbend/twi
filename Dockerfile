# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/twi ./cmd/twi

FROM debian:bookworm-slim AS runtime

RUN groupadd --system --gid 10001 twi \
	&& useradd --system --uid 10001 --gid twi --home-dir /home/twi --create-home twi \
	&& mkdir -p /config /cache \
	&& chown -R twi:twi /config /cache /home/twi

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/twi /usr/local/bin/twi

ENV XDG_CONFIG_HOME=/config \
	XDG_CACHE_HOME=/cache \
	TERM=xterm-256color

USER twi:twi
WORKDIR /home/twi

ENTRYPOINT ["twi"]
CMD ["--help"]
