# IncidentOps Arena — container image.
#
# The build stage compiles a standard-library-only module (ADR-001), so it never
# reaches a module proxy: GOFLAGS=-mod=mod with an empty require block resolves
# nothing. The runtime stage carries the binary, a non-root user and nothing else.

FROM golang:1.24-alpine AS build

WORKDIR /src

# The module has no dependencies, so the whole source tree is the build context and
# there is no separate dependency-download layer to cache.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=0.1.0-mvp
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOPROXY=off

RUN go build -ldflags="-s -w -X main.Version=${VERSION}" -o /out/arena   ./cmd/arena \
 && go build -ldflags="-s -w"                            -o /out/evalctl ./cmd/evalctl \
 && go build -ldflags="-s -w"                            -o /out/faultctl ./cmd/faultctl \
 && go build -ldflags="-s -w"                            -o /out/sddctl  ./cmd/sddctl

# ---------------------------------------------------------------------------

FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget \
 && addgroup -g 10001 -S arena \
 && adduser  -u 10001 -S -G arena arena \
 && mkdir -p /data && chown arena:arena /data

COPY --from=build /out/arena    /usr/local/bin/arena
COPY --from=build /out/evalctl  /usr/local/bin/evalctl
COPY --from=build /out/faultctl /usr/local/bin/faultctl
COPY --from=build /out/sddctl   /usr/local/bin/sddctl

USER arena
WORKDIR /data

ENV ARENA_ADDR=:8080 \
    ARENA_STORE=file \
    ARENA_DATA=/data

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["arena"]
CMD ["serve"]
