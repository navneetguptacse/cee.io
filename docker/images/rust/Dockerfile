FROM rust:1.75-slim

RUN useradd -m -u 1001 -s /bin/bash runner \
    && mkdir -p /box && chown runner:runner /box

WORKDIR /box
USER runner

CMD ["sleep", "infinity"]

