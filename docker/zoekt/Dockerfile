FROM golang:1.22-bookworm AS build

WORKDIR /src
RUN git clone --depth=1 https://github.com/sourcegraph/zoekt.git .
RUN CGO_ENABLED=0 go build -o /out/zoekt-index ./cmd/zoekt-index
RUN CGO_ENABLED=0 go build -o /out/zoekt-webserver ./cmd/zoekt-webserver

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/zoekt-index /usr/local/bin/zoekt-index
COPY --from=build /out/zoekt-webserver /usr/local/bin/zoekt-webserver

WORKDIR /data

