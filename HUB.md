# CEE | Code Execution Engine

A high-performance, Judge0-compatible code execution engine and CLI built in Go.

[![Docker Image](https://img.shields.io/badge/docker-navneetguptacse%2Fcee-blue.svg?logo=docker&logoColor=white)](https://hub.docker.com/r/navneetguptacse/cee)
[![GHCR](https://img.shields.io/badge/ghcr.io-navneetguptacse%2Fcee.io-blue.svg?logo=github&logoColor=white)](https://github.com/navneetguptacse/cee.io/pkgs/container/cee.io)
[![Multi-Platform](https://img.shields.io/badge/platform-linux%2Famd64%20%7C%20linux%2Farm64-lightgrey.svg)](https://hub.docker.com/r/navneetguptacse/cee)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![API: Judge0 Compatible](https://img.shields.io/badge/API-Judge0%20Compatible-blue.svg)](https://judge0.com)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](https://github.com/navneetguptacse/cee.io/blob/main/LICENSE)

---

## Overview

CEE (Code Execution Engine) is a drop-in replacement for Judge0 built from scratch in Go. It provides ultra-low latency execution of untrusted user code with strict resource constraints, microsecond queue wakeups, role-based API key management, and full Judge0 API compatibility.

Multi-platform images (`linux/amd64` and `linux/arm64`) are published to both Docker Hub and GitHub Container Registry (GHCR).

- **GitHub Repository**: [github.com/navneetguptacse/cee.io](https://github.com/navneetguptacse/cee.io)
- **User Guide**: [GUIDE.md](https://github.com/navneetguptacse/cee.io/blob/main/GUIDE.md)

---

## Quick Start (Run in 5 Seconds)

Pull and run the CEE container with a single command (no external Redis or database required):

### From Docker Hub:

```bash
docker run -d -p 3000:3000 --name cee navneetguptacse/cee:latest
```

### From GitHub Container Registry (GHCR):

```bash
docker run -d -p 3000:3000 --name cee ghcr.io/navneetguptacse/cee.io:latest
```

The container starts with an embedded in-memory channel queue and begins accepting submissions immediately.

---

## Test Execution

Send a synchronous code execution request using `curl`:

```bash
curl -X POST "http://localhost:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -d '{
    "language_id": 71,
    "source_code": "print(21 * 2)"
  }'
```

Response:

```json
{
  "token": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": {
    "id": 3,
    "description": "Accepted"
  },
  "stdout": "42\n",
  "stderr": null,
  "compile_output": null,
  "time": "0.045",
  "wall_time": "0.045",
  "exit_code": 0
}
```

---

## Built-in CLI Client Distribution

The CEE server container automatically serves precompiled static CLI binaries and an automated install script:

```bash
# Install the native cee CLI from your running container:
curl -fsSL http://localhost:3000/install.sh | bash

# Authenticate CLI with your server:
cee auth master <YOUR_TOKEN> -u http://localhost:3000

# Execute code remotely using the CLI:
cee submit -c "print('Hello from CEE CLI!')" -l py
```

---

## Supported Programming Languages

| Language ID         | Language   | Compiler / Runtime                |
| :------------------ | :--------- | :-------------------------------- |
| **71** (or 92)      | Python     | Python 3.8 / 3.11                 |
| **63** (or 93, 102) | JavaScript | Node.js 18 / 22 LTS               |
| **74** (or 94)      | TypeScript | TypeScript 5.0                    |
| **50**              | C          | GCC 9.2                           |
| **54**              | C++        | G++ 9.2                           |
| **60** (or 95)      | Go         | Go 1.22+                          |
| **62**              | Java       | OpenJDK 17                        |
| **73**              | Rust       | Rustc 1.75                        |
| **46**              | Bash       | Bash 5.0                          |
| **89**              | Multi-file | ZIP archive with `run` entrypoint |

---

## Production Security & Authentication

In production environments exposed to the internet, you **must set `AUTH_TOKEN`** to protect the API and enable role-based key management.

### 1. Generate a Cryptographically Secure Token

Generate a 64-character token in your terminal:

```bash
AUTH_TOKEN=$(openssl rand -hex 32)
echo "Token: $AUTH_TOKEN"
```

### 2. Run Container with Authentication

Pass the generated token using `-e AUTH_TOKEN`:

```bash
docker run -d -p 3000:3000 \
  -e AUTH_TOKEN="$AUTH_TOKEN" \
  -e MAX_WORKERS=8 \
  --name cee navneetguptacse/cee:latest
```

### 3. Authenticate Client Requests

When `AUTH_TOKEN` is configured, clients must include the `X-Auth-Token` HTTP header in every request:

```bash
curl -X POST "http://your-server-ip:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $AUTH_TOKEN" \
  -d '{
    "language_id": 71,
    "source_code": "print(\"Authenticated!\")"
  }'
```

If the header is missing or unauthorized, CEE returns `HTTP 401 Unauthorized` (or `HTTP 403 Forbidden`) with non-leaking security errors.

---

## Configuration & Environment Variables

| Variable         | Default   | Description                                                                        |
| :--------------- | :-------- | :--------------------------------------------------------------------------------- |
| `PORT`           | `3000`    | HTTP port to listen on                                                             |
| `AUTH_TOKEN`     | _empty_   | **Mandatory for production.** Initial Master AUTH token required for API access    |
| `METRICS_TOKEN`  | _empty_   | Optional token dedicated to scraping Prometheus `/metrics`                         |
| `MAX_WORKERS`    | `4`       | Number of concurrent execution workers                                             |
| `EXECUTOR_TYPE`  | `process` | Sandbox engine: `process`, `docker`, or `isolate`                                  |
| `REDIS_URL`      | _empty_   | Redis connection string (e.g. `redis://redis:6379`). Uses in-memory queue if empty |
| `RATE_LIMIT_RPS` | `100`     | Sliding-window requests per second limit per IP                                    |

---

## Production Deployment with Docker Compose & Redis

For high-throughput production clusters, create a `.env` file and launch CEE with Redis:

```bash
# 1. Create production environment configuration
cat > .env <<EOF
AUTH_TOKEN=$(openssl rand -hex 32)
METRICS_TOKEN=$(openssl rand -hex 16)
PORT=3000
MAX_WORKERS=8
EOF

# 2. Start stack
docker compose up -d
```

Example `docker-compose.yml`:

```yaml
version: "3.8"

services:
  cee:
    image: navneetguptacse/cee:latest
    ports:
      - "${PORT:-3000}:3000"
    environment:
      - AUTH_TOKEN=${AUTH_TOKEN}
      - METRICS_TOKEN=${METRICS_TOKEN}
      - REDIS_URL=redis://redis:6379
      - MAX_WORKERS=${MAX_WORKERS:-8}
    depends_on:
      - redis
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    restart: unless-stopped
```

---

## Documentation & Source Code

- **GitHub Repository**: [github.com/navneetguptacse/cee.io](https://github.com/navneetguptacse/cee.io)
- **Step-by-Step User Guide**: [GUIDE.md](https://github.com/navneetguptacse/cee.io/blob/main/GUIDE.md)
- **Comprehensive Test Suite**: [TEST.md](https://github.com/navneetguptacse/cee.io/blob/main/TEST.md)
- **License**: MIT License
