# Multi-stage build. Base images are pinned by digest; Renovate keeps the tag
# and digest current.
# Builder runs on the build platform and cross-compiles (no emulation needed).
FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine@sha256:ae52b5bc6ed045a9ed26ab03dc19327a802a9cc9aa8b27ed57f5c6b60579b0bb AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download and verify dependencies against go.sum
RUN go mod download && go mod verify

# Copy source
COPY . .

# Build arguments
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build static binary (CGO_ENABLED=0)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -mod=readonly \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o semrel \
    ./cmd/semrel

# Final stage: alpine (distroless/static has no shell; entrypoint.sh needs sh).
# Runs as an unprivileged user by default. action.yml additionally runs the
# container with the runner's own uid/gid so it can write the mounted
# workspace (.git) and $GITHUB_OUTPUT.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

COPY --from=builder /app/semrel /semrel
COPY entrypoint.sh /entrypoint.sh

USER 65532:65532
ENTRYPOINT ["/entrypoint.sh"]
