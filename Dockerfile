# syntax=docker/dockerfile:1

# ---- Build stage -----------------------------------------------------------
# Runs on the build machine's native platform and cross-compiles for the
# target, which is much faster than emulating e.g. arm64.
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
# Reported by GET /api/health; CI passes the git commit SHA.
ARG VERSION=dev

WORKDIR /src

# Download dependencies first so this layer is cached between code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server

# ---- Runtime stage ---------------------------------------------------------
# Distroless static: no shell or package manager, runs as a non-root user.
# It includes CA certificates, needed for HTTPS calls to the Frankfurter API.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/server /server

ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/server"]
