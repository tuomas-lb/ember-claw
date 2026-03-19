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

# Stage 2: Runtime with development tools
FROM alpine:3.23

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    # Core utilities
    curl \
    jq \
    wget \
    bash \
    git \
    openssh-client \
    # Build tools
    make \
    gcc \
    musl-dev \
    # Python
    python3 \
    py3-pip \
    # Node.js
    nodejs \
    npm \
    # Go (latest stable from Alpine packages)
    go

# Create non-root user matching PVC ownership expectations
RUN addgroup -g 1000 picoclaw && \
    adduser -D -u 1000 -G picoclaw -h /home/picoclaw -s /bin/bash picoclaw

COPY --from=builder /sidecar /usr/local/bin/sidecar

# Allow pip install without virtual env (safe in container)
ENV PIP_BREAK_SYSTEM_PACKAGES=1

# Set up Go environment for picoclaw user
ENV GOPATH=/home/picoclaw/go
ENV PATH="/home/picoclaw/go/bin:/usr/local/go/bin:${PATH}"

USER picoclaw
WORKDIR /home/picoclaw

EXPOSE 50051 8080

ENTRYPOINT ["sidecar"]
