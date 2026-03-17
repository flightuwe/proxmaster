FROM golang:1.22

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      iproute2 \
      jq \
      wireguard-tools && \
    rm -rf /var/lib/apt/lists/*

