FROM eclipse-temurin:17-jdk-jammy

RUN useradd -m -u 1001 -s /bin/bash runner \
    && mkdir -p /box && chown runner:runner /box

WORKDIR /box
USER runner

CMD ["sleep", "infinity"]

