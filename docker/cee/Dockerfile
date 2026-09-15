FROM golang:alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/cee ./cmd/cee

FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl bash unzip \
    gcc g++ make \
    python3 \
    openjdk-17-jdk-headless \
    nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1001 -s /bin/bash runner

WORKDIR /app
COPY --from=builder /bin/cee /usr/local/bin/cee

EXPOSE 3000

USER runner

ENTRYPOINT ["cee"]
CMD ["server", "--port", "3000"]
