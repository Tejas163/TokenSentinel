FROM rust:1.94-slim-bookworm AS builder
RUN apt-get update && apt-get install -y pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y libssl3 ca-certificates && rm -rf /var/lib/apt/lists/* && groupadd -r appgroup && useradd -r -g appgroup appuser
COPY --from=builder /app/target/release/proxyops-proxy /usr/local/bin/
EXPOSE 3000
USER appuser
CMD ["proxyops-proxy"]
