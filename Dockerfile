# syntax=docker/dockerfile:1.7

# Build stage: compile a static terradrift binary against go.mod's pinned
# toolchain. CGO is disabled so the resulting binary works on the
# distroless-style alpine runtime stage without libc symbol surprises.
FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/terradrift \
    .

# Runtime stage: minimal alpine plus the two utilities the entrypoint
# needs to talk to the GitHub API (curl) and JSON-encode comment bodies
# (jq). ca-certificates is required for HTTPS to api.github.com and
# remote state backends (s3:// / https://).
FROM alpine:3.19

LABEL org.opencontainers.image.title="terradrift"
LABEL org.opencontainers.image.description="GitHub Action for Terraform drift detection"
LABEL org.opencontainers.image.source="https://github.com/esanchezm/terradrift"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates curl jq

COPY --from=builder /out/terradrift /usr/local/bin/terradrift
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
