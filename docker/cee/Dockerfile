FROM golang:latest AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/cee ./cmd/cee

FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash unzip gnupg \
    gcc g++ make \
    python3 \
    openjdk-17-jdk-headless \
    rustc \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g typescript \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/go /usr/local/go
COPY --from=builder /bin/cee /usr/local/bin/cee

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOCACHE=/tmp/go-cache
ENV GOPATH=/tmp/go

RUN mkdir -p /tmp/go-cache /tmp/go && chmod -R 777 /tmp/go-cache /tmp/go

RUN useradd -m -u 1001 -s /bin/bash runner

WORKDIR /app

EXPOSE 3000

USER runner

ENTRYPOINT ["cee"]
CMD ["server", "--port", "3000"]
