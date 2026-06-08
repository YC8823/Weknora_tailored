# Build stage: compile Go binary
FROM golang:1.24-bookworm AS builder

WORKDIR /app

ARG GOPRIVATE_ARG
ARG GOPROXY_ARG
ARG GOSUMDB_ARG=off
ARG APK_MIRROR_ARG

ENV GOPRIVATE=${GOPRIVATE_ARG}
ENV GOPROXY=${GOPROXY_ARG}
ENV GOSUMDB=${GOSUMDB_ARG}

RUN sed -i "s@deb.debian.org@${APK_MIRROR_ARG:-mirrors.tuna.tsinghua.edu.cn}@g" /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y git build-essential libsqlite3-dev

RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/download cmd/download
RUN go run cmd/download/duckdb/duckdb.go
COPY . .

ARG VERSION_ARG
ARG COMMIT_ID_ARG
ARG BUILD_TIME_ARG
ARG GO_VERSION_ARG

ENV VERSION=${VERSION_ARG}
ENV COMMIT_ID=${COMMIT_ID_ARG}
ENV BUILD_TIME=${BUILD_TIME_ARG}
ENV GO_VERSION=${GO_VERSION_ARG}

RUN --mount=type=cache,target=/go/pkg/mod make build-prod
RUN --mount=type=cache,target=/go/pkg/mod cp -r /go/pkg/mod/github.com/yanyiwu/ /app/yanyiwu/

# Final stage: use official pre-built image, only swap the binary.
# This avoids rebuilding apt packages from scratch in a Chinese network environment.
FROM wechatopenai/weknora-app:${WEKNORA_VERSION:-latest}

# Overwrite the binary with our custom-built version (contains vision-augment feature)
COPY --from=builder /app/WeKnora /app/WeKnora
COPY --from=builder /app/yanyiwu/ /go/pkg/mod/github.com/yanyiwu/

EXPOSE 8080

ENTRYPOINT ["./scripts/docker-entrypoint.sh"]
CMD ["./WeKnora"]