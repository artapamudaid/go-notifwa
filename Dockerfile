# ─────────────────────────────────────────────────────────
# Stage 1 — Builder
# Gunakan golang:bookworm (Debian) bukan alpine agar CGO
# (go-sqlite3) bisa di-compile dengan gcc penuh tanpa masalah
# musl/glibc mismatch.
# ─────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Install gcc & build tools (sudah tersedia di bookworm, tapi
# pastikan sqlite3 dev headers ada)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy dependency files dulu (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy semua source code
COPY . .

# Pastikan go.sum sinkron di dalam container
RUN go mod tidy

# Build dengan CGO enabled
# Tidak pakai -a dan -installsuffix cgo karena tidak diperlukan
# di lingkungan Debian yang sudah punya glibc native
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o main .

# ─────────────────────────────────────────────────────────
# Stage 2 — Runtime
# Gunakan debian:bookworm-slim agar binary glibc-linked bisa
# berjalan (tidak bisa pakai alpine karena binary butuh glibc)
# ─────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install runtime deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary dari builder stage
COPY --from=builder /app/main .

# Optimize Go Garbage Collection untuk container (limit 250 MB)
ENV GOMEMLIMIT=250MiB

EXPOSE 8088

CMD ["./main"]
