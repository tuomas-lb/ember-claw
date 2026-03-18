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

# Stage 2: Minimal runtime
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user matching PVC ownership expectations
RUN addgroup -g 1000 picoclaw && \
    adduser -D -u 1000 -G picoclaw -h /home/picoclaw picoclaw

COPY --from=builder /sidecar /usr/local/bin/sidecar

USER picoclaw

EXPOSE 50051

ENTRYPOINT ["sidecar"]
