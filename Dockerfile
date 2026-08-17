# Stage 1: Builder
FROM ubuntu:22.04 AS builder

ARG GOLANG_VERSION=1.21.5
ARG CLANG_VERSION=14

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    ca-certificates \
    git \
    build-essential \
    clang-${CLANG_VERSION} \
    llvm-${CLANG_VERSION} \
    libelf-dev \
    libpcap-dev \
    libbpf-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN wget -q https://go.dev/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GOLANG_VERSION}.linux-amd64.tar.gz && \
    rm go${GOLANG_VERSION}.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}" \
    CLANG="clang-${CLANG_VERSION}"

# Install bpftool (static build) to generate vmlinux.h from kernel BTF
ARG BPFTOOL_VERSION=v7.4.0
RUN wget -q https://github.com/libbpf/bpftool/releases/download/${BPFTOOL_VERSION}/bpftool-${BPFTOOL_VERSION}-amd64.tar.gz && \
    tar -xzf bpftool-${BPFTOOL_VERSION}-amd64.tar.gz -C /usr/local/bin bpftool && \
    rm bpftool-${BPFTOOL_VERSION}-amd64.tar.gz && \
    chmod +x /usr/local/bin/bpftool

WORKDIR /build

# Copy source code
COPY . .

# Build eBPF programs
RUN mkdir -p /build/bin && \
    bpftool btf dump file /sys/kernel/btf/vmlinux format c > pkg/ebpf/vmlinux.h && \
    ${CLANG} -O2 -target bpf -c pkg/ebpf/program.c -o /build/bin/program.o

# Build Go binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.Version=${VERSION:-dev}" \
    -o /build/bin/traffic-collector \
    ./cmd/collector

# Stage 2: Runtime
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libpcap0.8 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 collector

# Copy binaries from builder
COPY --from=builder /build/bin/traffic-collector /usr/local/bin/
COPY --from=builder /build/bin/program.o /etc/traffic-collector/

# Create config directory
RUN mkdir -p /etc/traffic-collector && \
    chown -R collector:collector /etc/traffic-collector

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/traffic-collector"]
