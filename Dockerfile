# The OurTaikoNet server: the API of net.ourtaiko.org and of the three old addresses. The
# website (web/) is not in the image; the server serves it from WEB_DIR, where its own
# releases are put (deploy/README.md).

# TJAParser's command-line tool: the game's own parser, which reads every uploaded chart's
# blocks, hashes them and lists the notes the difficulty constants are computed from. Pin a tag
# whose hashes match the game's (OurTaikoPlay Packages/manifest.json).
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0-alpine AS tjaparser
ARG TARGETARCH
ARG TJAPARSER_REF=v0.13.0
RUN apk add --no-cache git \
    && git clone --depth 1 --branch "$TJAPARSER_REF" https://github.com/OurTaiko/TJAParser.git /src
WORKDIR "/src/dotnet~"
# Self-contained, single-file and trimmed: the options live in tjaparser.csproj.
RUN rid=linux-musl-x64; [ "$TARGETARCH" = "arm64" ] && rid=linux-musl-arm64; \
    dotnet publish tjaparser -c Release -r "$rid" -o /out \
    && /out/tjaparser version

# Everything scripts/test.sh needs, with the tools the release image has. Not part of the
# release: `docker build --target test` makes it, for CI and for trying the suite as the
# server will run the code.
FROM golang:1.26-alpine AS test
RUN apk add --no-cache ffmpeg libwebp-tools libstdc++ libgcc
COPY --from=tjaparser /out/tjaparser /usr/local/bin/tjaparser
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["scripts/test.sh"]

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY db/ ./db/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/ourtaiko-net ./cmd/server \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/ourtaiko-rooms ./cmd/rooms

FROM alpine:3.23
RUN apk add --no-cache ca-certificates ffmpeg libwebp-tools curl libstdc++ libgcc \
    && addgroup -g 10001 app && adduser -D -u 10001 -G app app \
    && mkdir -p /data/files && chown -R app:app /data
COPY --from=build /out/ourtaiko-net /out/ourtaiko-rooms /usr/local/bin/
COPY --from=tjaparser /out/tjaparser /usr/local/bin/tjaparser
USER 10001:10001
ENV LISTEN_ADDR=0.0.0.0:8080 STORAGE_DIR=/data/files
# The proxy in front reaches a published port from the container's default gateway, the host;
# it is trusted besides TRUSTED_PROXY_CIDRS, however the networks change (internal/web).
ENV TRUSTED_PROXY_GATEWAY=true
# 8080: the API and the website. 8081: Multi-Enso, the game's online rooms (gRPC in clear
# text), served by the same image with the entrypoint ourtaiko-rooms (deploy/compose.production.yml).
EXPOSE 8080 8081
ENTRYPOINT ["ourtaiko-net"]
