# ── Stage 1: Go SDK (for COPY into runtime) ───────────────────────────────────
FROM golang:1.24.7-bookworm AS go-sdk

# ── Stage 2: build tgconn ─────────────────────────────────────────────────────
FROM golang:1.24.7-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
      -ldflags "-s -w -X github.com/cx009/tgconn/cmd.version=${VERSION}" \
      -o /tgconn .

# ── Stage 3: runtime ──────────────────────────────────────────────────────────
# Base: Python 3.13.11 (Debian bookworm-slim).
# Adds: Go 1.24.7 (copied from go-sdk) + Node.js 24.7.0 (official tarball).
FROM python:3.13.11-slim-bookworm

ARG NODE_VERSION=24.7.0

# System deps + Node.js (arch-aware: supports amd64 and arm64)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl xz-utils git \
    && rm -rf /var/lib/apt/lists/* \
    && ARCH=$(dpkg --print-architecture) \
    && case "${ARCH}" in \
         amd64) NODE_ARCH="x64"   ;; \
         arm64) NODE_ARCH="arm64" ;; \
         *)     echo "Unsupported arch: ${ARCH}" && exit 1 ;; \
       esac \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
       | tar -xJ -C /usr/local --strip-components=1

# Go 1.24.7 — GOPATH under user home, set after user creation
COPY --from=go-sdk /usr/local/go /usr/local/go
ENV GOROOT=/usr/local/go

# Claude CLI (installed globally before user switch)
RUN npm install -g @anthropic-ai/claude-code

# tgconn binary
COPY --from=builder /tgconn /usr/local/bin/tgconn

# Non-root user — required because claude CLI refuses --dangerously-skip-permissions as root
RUN useradd -m -u 1000 -s /bin/bash tgconn \
    && mkdir -p /home/tgconn/go
ENV HOME=/home/tgconn
ENV GOPATH=/home/tgconn/go
ENV PATH="/usr/local/go/bin:/home/tgconn/go/bin:${PATH}"

# Smoke-test all three runtimes at build time
RUN go version && node --version && python3 --version

USER tgconn

# /workspace is the project directory tgconn will operate in.
# Mount your project here: -v $(pwd):/workspace
WORKDIR /workspace

ENTRYPOINT ["tgconn"]
CMD ["--help"]
