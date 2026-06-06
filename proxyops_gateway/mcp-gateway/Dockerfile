FROM rust:1.94-slim-bookworm AS builder
RUN apt-get update && apt-get install -y pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY Cargo.toml Cargo.lock* ./
COPY src/ src/

RUN cargo build --release

FROM debian:bookworm-slim
COPY --from=builder /app/target/release/mcp-gateway /usr/local/bin/mcp-gateway
EXPOSE 3010 3011 3012
CMD ["mcp-gateway"]
