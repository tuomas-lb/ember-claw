# Stage 1: Build sidecar binary
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Cache module downloads before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build for linux/amd64
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /sidecar ./cmd/sidecar

# Stage 2: Runtime with development tools (Debian for glibc compatibility with bun/backlog.md)
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    # Core utilities
    curl \
    jq \
    wget \
    bash \
    git \
    openssh-client \
    unzip \
    # Build tools
    make \
    gcc \
    libc6-dev \
    # Python
    python3 \
    python3-pip \
    python3-venv \
    # Node.js (via nodesource for recent version)
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN curl -fsSL https://go.dev/dl/go1.24.1.linux-amd64.tar.gz | tar -C /usr/local -xz

# Install Bun
RUN curl -fsSL https://bun.sh/install | bash && \
    mv /root/.bun/bin/bun /usr/local/bin/bun && \
    ln -s /usr/local/bin/bun /usr/local/bin/bunx

# Create non-root user matching PVC ownership expectations
RUN groupadd -g 1000 picoclaw && \
    useradd -u 1000 -g picoclaw -m -d /home/picoclaw -s /bin/bash picoclaw

COPY --from=builder /sidecar /usr/local/bin/sidecar

# Allow pip install without virtual env (safe in container)
ENV PIP_BREAK_SYSTEM_PACKAGES=1

# Pre-install commonly needed Python packages
RUN pip install --no-cache-dir --break-system-packages requests beautifulsoup4 pyyaml

# Install Backlog.md task manager and CalDAV MCP server (used by PicoClaw as MCP tools)
RUN npm install -g backlog.md caldav-mcp

# Set up Go environment for picoclaw user
ENV GOPATH=/home/picoclaw/go
# pip installs to PVC so packages survive pod restarts
ENV PYTHONUSERBASE=/home/picoclaw/.picoclaw/python
ENV PATH="/home/picoclaw/.picoclaw/python/bin:/home/picoclaw/go/bin:/usr/local/go/bin:${PATH}"
ENV PIP_USER=1

USER picoclaw
WORKDIR /home/picoclaw

EXPOSE 50051 8080

ENTRYPOINT ["sidecar"]
