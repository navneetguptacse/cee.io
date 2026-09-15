# CEE | Code Execution Engine

A high-performance, Judge0-compatible code execution engine and CLI built in Go.

[![Docker Image](https://img.shields.io/badge/docker-navneetguptacse%2Fcee-blue.svg?logo=docker&logoColor=white)](https://hub.docker.com/r/navneetguptacse/cee)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![API: Judge0 Compatible](https://img.shields.io/badge/API-Judge0%20Compatible-blue.svg)](https://judge0.com)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](https://github.com/navneetguptacse/cee.io/blob/main/LICENSE)

---

## Overview

CEE (Code Execution Engine) is a drop-in replacement for Judge0 built from scratch in Go. It provides ultra-low latency execution of untrusted user code with strict resource constraints, microsecond queue wakeups, and full Judge0 API compatibility.

GitHub Repository: [github.com/navneetguptacse/cee.io](https://github.com/navneetguptacse/cee.io)

---

## Quick Start (Run in 5 Seconds)

Start the CEE container with a single command (no external Redis or database required):

```bash
docker run -d -p 3000:3000 --name cee navneetguptacse/cee:latest
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

## Supported Programming Languages

| Language ID         | Language   | Compiler / Runtime                |
| :------------------ | :--------- | :-------------------------------- |
| **71** (or 92)      | Python     | Python 3.8 / 3.11                 |
| **63** (or 93, 102) | JavaScript | Node.js 18 / 22                   |
| **74** (or 94)      | TypeScript | TypeScript 5.0                    |
| **50**              | C          | GCC 9.2                           |
| **54**              | C++        | G++ 9.2                           |
| **60** (or 95)      | Go         | Go 1.22+                          |
| **62**              | Java       | OpenJDK 17                        |
| **73**              | Rust       | Rustc 1.75                        |
| **46**              | Bash       | Bash 5.0                          |
| **89**              | Multi-file | ZIP archive with `run` entrypoint |

---

## Configuration & Environment Variables

Configure CEE by passing environment variables with `-e`:

```bash
docker run -d -p 3000:3000 \
  -e AUTH_TOKEN=your-secret-token \
  -e MAX_WORKERS=8 \
  --name cee navneetguptacse/cee:latest
```

| Variable         | Default   | Description                                                                        |
| :--------------- | :-------- | :--------------------------------------------------------------------------------- |
| `PORT`           | `3000`    | HTTP port to listen on                                                             |
| `AUTH_TOKEN`     | _empty_   | Optional token. When set, clients must pass `X-Auth-Token`                         |
| `MAX_WORKERS`    | `4`       | Number of concurrent execution workers                                             |
| `EXECUTOR_TYPE`  | `process` | Sandbox engine: `process`, `docker`, or `isolate`                                  |
| `REDIS_URL`      | _empty_   | Redis connection string (e.g. `redis://redis:6379`). Uses in-memory queue if empty |
| `RATE_LIMIT_RPS` | `100`     | Sliding-window requests per second limit per IP                                    |

---

## Production Deployment with Docker Compose

For high-throughput production clusters, run CEE with Redis:

```yaml
version: "3.8"

services:
  cee:
    image: navneetguptacse/cee:latest
    ports:
      - "3000:3000"
    environment:
      - REDIS_URL=redis://redis:6379
      - MAX_WORKERS=8
      - AUTH_TOKEN=your-secret-token
    depends_on:
      - redis
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    restart: unless-stopped
```

Start the stack:

```bash
docker compose up -d
```

---

## Documentation & Source Code

- **GitHub Repository**: [github.com/navneetguptacse/cee.io](https://github.com/navneetguptacse/cee.io)
- **Step-by-Step User Guide**: [GUIDE.md](https://github.com/navneetguptacse/cee.io/blob/main/GUIDE.md)
- **License**: MIT License
