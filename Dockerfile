# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o nowtv-simulator .

# ── Stage 2: minimal runtime image ────────────────────────────────────────────
FROM scratch

COPY --from=builder /build/nowtv-simulator /nowtv-simulator

# ECP HTTP port
EXPOSE 8060

# SSDP multicast — requires --network host on Linux for discovery to work
EXPOSE 1900/udp

ENTRYPOINT ["/nowtv-simulator"]
