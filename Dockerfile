# Stage 1: Build Svelte frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app

COPY frontend/package*.json ./frontend/
RUN cd frontend && npm install

COPY frontend/ ./frontend/
RUN cd frontend && npm run build

# Stage 2: Build Go backend binary
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy built frontend assets into internal/api package for embedding
COPY --from=frontend-builder /app/internal/api/dashboard.html ./internal/api/dashboard.html
COPY --from=frontend-builder /app/internal/api/kerob_logo.png ./internal/api/kerob_logo.png

ARG VERSION=v0.1.1
ARG COMMIT=dev
ARG BUILD_DATE=""

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-w -s -X kerob/internal/version.Version=${VERSION} -X kerob/internal/version.Commit=${COMMIT} -X kerob/internal/version.BuildDate=${BUILD_DATE}" \
    -o /app/kerob ./cmd/server

# Stage 3: Runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S kerob && adduser -S kerob -G kerob

COPY --from=builder /app/kerob /app/kerob

USER kerob

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/kerob"]
