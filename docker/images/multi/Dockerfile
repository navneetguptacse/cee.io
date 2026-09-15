FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash unzip \
    gcc g++ make \
    python3 \
    openjdk-17-jdk-headless \
    nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1001 -s /bin/bash runner \
    && mkdir -p /box && chown runner:runner /box

WORKDIR /box
USER runner

CMD ["sleep", "infinity"]

