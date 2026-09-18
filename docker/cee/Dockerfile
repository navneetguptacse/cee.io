FROM --platform=$BUILDPLATFORM golang:latest AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o /bin/cee ./cmd/cee
RUN mkdir -p /dist && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o /dist/cee-darwin-arm64 ./cmd/cee && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o /dist/cee-darwin-amd64 ./cmd/cee && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /dist/cee-linux-amd64 ./cmd/cee && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /dist/cee-linux-arm64 ./cmd/cee && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /dist/cee-windows-amd64.exe ./cmd/cee

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
COPY --from=builder /dist /app/bin/dist
COPY --from=builder /src/install.sh /app/install.sh
RUN chmod -R 755 /app/bin/dist /app/install.sh

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
