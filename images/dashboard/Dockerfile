# Stage 1: Build frontend
FROM node:22-slim AS frontend
WORKDIR /app/web
COPY dashboard/web/package*.json ./
RUN npm ci
COPY dashboard/web/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.26-bookworm AS backend
WORKDIR /app
COPY dashboard/go.mod dashboard/go.sum ./
RUN go mod download
COPY dashboard/ ./
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /dashboard ./cmd/server

# Stage 3: Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=backend /dashboard /usr/local/bin/dashboard
EXPOSE 8090
USER 1000
CMD ["dashboard"]
