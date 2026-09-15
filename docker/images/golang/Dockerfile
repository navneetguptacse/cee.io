FROM golang:1.22-alpine

RUN adduser -D -u 1001 runner \
    && mkdir -p /box && chown runner:runner /box

WORKDIR /box
USER runner

CMD ["sleep", "infinity"]

