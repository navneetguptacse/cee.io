# CEE | Code Execution Engine

**A high-performance, Judge0-compatible code execution engine and CLI built in Go.**

[![Docker Image](https://img.shields.io/badge/docker-navneetguptacse%2Fcee-blue.svg?logo=docker&logoColor=white)](https://hub.docker.com/r/navneetguptacse/cee)
[![GHCR](https://img.shields.io/badge/ghcr.io-navneetguptacse%2Fcee.io-blue.svg?logo=github&logoColor=white)](https://github.com/navneetguptacse/cee.io/pkgs/container/cee.io)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![API: Judge0 Compatible](https://img.shields.io/badge/API-Judge0%20Compatible-blue.svg)](https://judge0.com)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey.svg)]()
[![User Guide](https://img.shields.io/badge/guide-step--by--step-orange.svg)](GUIDE.md)

> Looking for a beginner-friendly tutorial? Read the complete [User Guide (GUIDE.md)](GUIDE.md). For Docker Hub & GHCR container documentation, see [HUB.md](HUB.md). For end-to-end verification scenarios, see [TEST.md](TEST.md).

---

## What is CEE?

CEE (Code Execution Engine) is a self-hosted, drop-in replacement for Judge0 built from scratch in Go. It securely runs user-submitted code in isolated environments with strict resource constraints and returns execution output, metrics, and exit statuses with sub-millisecond scheduling overhead.

### Key Features

- **Judge0 API Compatible** - Complete drop-in replacement matching Judge0 endpoints and data schemas.
- **Microsecond Synchronous Wakeup** - Zero-polling instant response on `?wait=true` requests using Go channels and Redis Pub/Sub.
- **Tri-Engine Sandboxing** - Supports Linux Isolate (cgroups v2), Docker containers, and Native Process sandboxing.
- **Dual Queue Architecture** - Embedded in-memory channel queue for single-node / CLI use, or distributed Redis queue for multi-node clusters.
- **Integrated CLI (`cee`)** - One-shot local compilation and execution (`cee run`), server management (`cee server`), and diagnostics (`cee test`).
- **Comprehensive Language Support** - Python, JavaScript, TypeScript, C, C++, Java, Go, Rust, Bash, and Multi-file archives.
- **Defense-in-Depth Security** - Pre-execution static security scanner, SSRF validation on callback URLs, and command argument sanitization.
- **Production Ready** - Caddy reverse proxy with automatic SSL, Prometheus metrics, and Docker Compose configs.

---

## How It Works

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Frontend   │────▶│   CEE API    │────▶│ Queue Engine │────▶│   Workers    │
│   or CLI     │     │   (Go HTTP)  │     │Memory / Redis│     │ (Goroutines) │
└──────────────┘     └──────────────┘     └──────────────┘     └──────┬───────┘
                                                                      │
                                                    ┌─────────────────┴─────────────────┐
                                                    │                                   │
                                             ┌──────▼──────┐                     ┌──────▼──────┐
                                             │   Isolate   │                     │   Docker    │
                                             │  (cgroups)  │         OR          │ (container) │
                                             └──────┬──────┘                     └──────┬──────┘
                                                    │                                   │
                                                    └─────────────┬─────────────────────┘
                                                                  │
                                                    ┌─────────────▼─────────────┐
                                                    │  Sandboxed Code Execution │
                                                    │  (isolated, time-limited) │
                                                    └───────────────────────────┘
```

---

## Running CEE

Pick the mode that matches your setup:

| Mode                        | Prerequisites      | URL                     | TLS       | Build?           |
| :-------------------------- | :----------------- | :---------------------- | :-------- | :--------------- |
| **1. CLI Only**             | Go installed       | Local terminal          | None      | `go build`       |
| **2. Standalone Server**    | Go binary          | `http://localhost:3000` | None      | `go build`       |
| **3. Docker Compose (Dev)** | Docker Desktop     | `http://localhost:3000` | None      | `docker compose` |
| **4. Server (IP only)**     | Linux VPS + Docker | `http://<server-ip>`    | None      | `docker compose` |
| **5. Server with Domain**   | VPS + DNS A record | `https://cee.io`        | Automatic | `docker compose` |

---

### Mode 1 — Using the `cee` CLI

CEE CLI can be installed without needing Go or build tools:

#### Install via One-Liner Script:

```bash
# From your self-hosted CEE server:
curl -fsSL http://<server-ip>/install.sh | bash

# Or directly from GitHub:
curl -fsSL https://raw.githubusercontent.com/navneetguptacse/cee.io/main/install.sh | bash
```

#### Install via Homebrew (macOS & Linux):

```bash
brew tap navneetguptacse/cee https://github.com/navneetguptacse/cee.io
brew install navneetguptacse/cee/cee
```

#### Install via NPM:

```bash
npm install -g cee-cli
```

#### Or Build and Install from Source:

```bash
cd cee.io
make build
sudo make install
```

#### Quick CLI Usage:

```bash
# Run inline code directly
cee run -c "print(100 * 5)" -l py
cee run "console.log(21 * 2)" -l js

# Pipe code via stdin
echo "print('hello from stdin')" | cee run -l py

# Run Python code directly
cee run script.py --stdin "Hello World"

# Run C++ code with automatic compilation
cee run solution.cpp --stdin "10 20" --expected "30"

# Run Go code
cee run main.go

# List all supported language IDs and compilers
cee languages

# Run self-diagnostic suite
cee test
```

---

### Mode 2 — Standalone Server (Zero External Dependencies)

CEE includes an embedded in-memory channel queue. You can run the entire server and worker system as a single static binary without needing Redis or Docker:

```bash
cd cee.io

# Start the server on port 3000
./bin/cee server --port 3000

# Check health
curl http://localhost:3000/health

# Submit Python code synchronously
curl -X POST "http://localhost:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -d '{"language_id": 71, "source_code": "print(21 * 2)"}'
```

---

### Mode 3 — Local Development with Docker Compose

Runs the CEE service alongside Redis:

**Prerequisites:** Docker Desktop

```bash
cd cee.io

# Start CEE + Redis
docker compose up -d

# Verify health
curl http://localhost:3000/health
```

The default development stack is configured with token `dev-token`:

```bash
curl -X POST "http://localhost:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: dev-token" \
  -d '{"language_id": 71, "source_code": "print(\"Hello from CEE!\")"}'
```

---

### Mode 4 — Production Server Without a Domain (IP Only, Plain HTTP)

For running on a VPS using its public IP address:

```bash
ssh root@<server-ip>
git clone https://github.com/navneetguptacse/cee.git /opt/cee
cd /opt/cee

# Create environment configuration
cat > .env <<EOF
AUTH_TOKEN=$(openssl rand -hex 32)
METRICS_TOKEN=$(openssl rand -hex 16)
DOMAIN=
WORKER_CPUS=2.0
WORKER_MEMORY=2G
EOF

# Start production stack
docker compose -f docker-compose.prod.yml up -d --build
```

Test from your local machine:

```bash
curl http://<server-ip>/health

curl -X POST "http://<server-ip>/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: <your-token>" \
  -d '{"language_id": 71, "source_code": "print(10 + 20)"}'
```

---

### Mode 5 — Production Server with Domain and HTTPS

Same stack as Mode 4, plus automatic SSL certification from Let's Encrypt managed by Caddy.

1. Point an `A` record at your server IP (e.g. `cee.io`).
2. Ensure ports `80` and `443` are open.
3. Set `DOMAIN` in `.env`:

```bash
cd /opt/cee

cat > .env <<EOF
AUTH_TOKEN=$(openssl rand -hex 32)
METRICS_TOKEN=$(openssl rand -hex 16)
DOMAIN=cee.io
WORKER_CPUS=2.0
WORKER_MEMORY=2G
EOF

docker compose -f docker-compose.prod.yml up -d --build
```

Verify HTTPS access:

```bash
curl https://cee.io/health
```

---

## CLI Reference

```text
Usage:
  cee [command]

Available Commands:
  run         Directly compile and execute a local source file or inline code
  server      Start the CEE API server and execution workers
  worker      Start a standalone CEE queue worker
  submit      Submit code to a running CEE server
  status      Fetch status of a submission by token
  languages   List all supported programming languages
  health      Check health of CEE API server
  test        Run self-diagnostic execution test suite
  auth        Log in and manage session credentials (master/guest/metrics)
  logout      Clear active session credentials
  token       Generate, list, inspect, and revoke API keys
  config      View or update local CLI configuration profiles
  connect     Interactive wizard to configure remote server and credentials
```

### Examples

**Run inline code directly:**

```bash
cee run -c "print(100 * 5)" -l py
cee run "console.log(21 * 2)" -l js
echo "print(42)" | cee run -l py
```

**Run file locally:**

```bash
cee run solution.cpp --stdin "10 20" --expected "30"
```

**Authenticate with CEE Server:**

```bash
# Log in as Master (full access & key generation)
cee auth master <master-key> -u https://cee.io

# Log in as Guest (execution & guest key creation)
cee auth guest <guest-key> -u https://cee.io

# Check active identity & granted permissions
cee auth status

# Log out
cee logout
```

**API Key Management (Master):**

```bash
# Generate a new guest key
cee token generate guest -d "Frontend Runner"

# Generate a master key
cee token generate master -d "Secondary Admin"

# List all active keys
cee token list

# Revoke a key
cee token revoke key_xxxx
```

**Submit to remote server:**

```bash
cee submit solution.py --url https://cee.io --token secret --wait
```

**Query submission status:**

```bash
cee status c506ea63-d1b1-4c2f-8e84-7aaf5a89d98d --url https://cee.io
```

---

## API Reference

### Installation & Binary Distribution (Public)

| Method | Endpoint              | Description                                          |
| :----- | :-------------------- | :--------------------------------------------------- |
| `GET`  | `/health`             | Service health status and uptime                     |
| `GET`  | `/install.sh`         | Automated shell installer script for client machines |
| `GET`  | `/download/:filename` | Direct download of precompiled static CLI binaries   |

### Authentication & Key Management

| Method   | Endpoint            | Access Level | Description                                         |
| :------- | :------------------ | :----------- | :-------------------------------------------------- |
| `POST`   | `/api/auth/login`   | Any Key      | Validate credentials and return granted permissions |
| `POST`   | `/api/auth/logout`  | Any Key      | End session confirmation                            |
| `GET`    | `/api/capabilities` | Any Key      | Inspect active role permissions and whoami identity |
| `POST`   | `/api/keys`         | Master/Guest | Generate a new API key (Master: all; Guest: guest)  |
| `GET`    | `/api/keys`         | Master Only  | List all managed API keys and metadata              |
| `DELETE` | `/api/keys/:id`     | Master Only  | Revoke an API key (with last-master lockout guard)  |

### Submissions

| Method   | Endpoint                        | Description                                        |
| :------- | :------------------------------ | :------------------------------------------------- |
| `POST`   | `/submissions`                  | Create asynchronous submission (returns `{token}`) |
| `POST`   | `/submissions?wait=true`        | Create submission and wait for completed result    |
| `GET`    | `/submissions/:token`           | Fetch execution result by token                    |
| `DELETE` | `/submissions/:token`           | Delete submission from cache                       |
| `POST`   | `/submissions/batch`            | Submit multiple submissions (up to 20)             |
| `GET`    | `/submissions/batch?tokens=a,b` | Fetch results for multiple tokens                  |

### System & Discovery

| Method | Endpoint         | Description                                                          |
| :----- | :--------------- | :------------------------------------------------------------------- |
| `GET`  | `/health`        | Service health status and uptime                                     |
| `GET`  | `/metrics`       | Prometheus metrics scrape endpoint (requires metrics or master auth) |
| `GET`  | `/languages`     | List active supported languages                                      |
| `GET`  | `/languages/all` | List all languages including archived                                |
| `GET`  | `/languages/:id` | Language detail, source file, compile/run commands                   |
| `GET`  | `/statuses`      | List all Judge0 status codes (1–14)                                  |
| `GET`  | `/about`         | Service version and maintainer metadata                              |
| `GET`  | `/system_info`   | CPU, RAM, and OS telemetry                                           |
| `GET`  | `/config_info`   | Active execution limits and defaults                                 |
| `GET`  | `/executor`      | Active sandbox engine and capabilities                               |
| `GET`  | `/workers`       | Active worker status                                                 |
| `GET`  | `/statistics`    | Queue depth and completion counts                                    |

---

## Supported Languages

| ID     | Language                | Default Compiler                           | Default Runner    |
| :----- | :---------------------- | :----------------------------------------- | :---------------- |
| **46** | Bash (5.0.17)           | None                                       | `bash script.sh`  |
| **50** | C (GCC 9.2.0)           | `gcc -O2 -o a.out main.c`                  | `./a.out`         |
| **54** | C++ (GCC 9.2.0)         | `g++ -O2 -std=c++17 -o a.out main.cpp`     | `./a.out`         |
| **60** | Go (1.22.0)             | `go build -o a.out main.go`                | `./a.out`         |
| **62** | Java (OpenJDK 17)       | `javac -cp .:/usr/local/lib/java/* *.java` | `java -cp . Main` |
| **63** | JavaScript (Node 18/22) | None                                       | `node script.js`  |
| **71** | Python (3.8.10)         | None                                       | `python3 main.py` |
| **73** | Rust (1.75.0)           | `rustc -O -o a.out main.rs`                | `./a.out`         |
| **74** | TypeScript (5.0.3)      | `tsc ts-main.ts --outDir .`                | `node ts-main.js` |
| **89** | Multi-file program      | `compile` / `compile.sh`                   | `run` / `run.sh`  |

_RapidAPI Compatibility Aliases:_ `92` (Python), `93` & `102` (JavaScript), `94` (TypeScript), `95` (Go).

---

## Status Codes

| ID     | Status                  | Description                                       |
| :----- | :---------------------- | :------------------------------------------------ |
| **1**  | In Queue                | Submission is waiting in queue                    |
| **2**  | Processing              | Submission is being executed                      |
| **3**  | Accepted                | Code executed and passed all test constraints     |
| **4**  | Wrong Answer            | Stdout did not match expected output              |
| **5**  | Time Limit Exceeded     | Process exceeded CPU or wall-time limit           |
| **6**  | Compilation Error       | Compilation failed or was rejected by static scan |
| **7**  | Runtime Error (SIGSEGV) | Segmentation fault                                |
| **8**  | Runtime Error (SIGXFSZ) | File size limit exceeded                          |
| **9**  | Runtime Error (SIGFPE)  | Floating point exception (divide by zero)         |
| **10** | Runtime Error (SIGABRT) | Program aborted                                   |
| **11** | Runtime Error (NZEC)    | Non-zero exit code                                |
| **12** | Runtime Error (Other)   | Other runtime errors                              |
| **13** | Internal Error          | Infrastructure error                              |
| **14** | Exec Format Error       | Binary format error                               |

---

## Configuration

| Variable                    | Default                       | Description                                          |
| :-------------------------- | :---------------------------- | :--------------------------------------------------- |
| `AUTH_TOKEN`                | Blank                         | Master API key / space-separated bootstrap tokens    |
| `METRICS_TOKEN`             | Blank                         | Dedicated scraping token for `/metrics`              |
| `AUTH_HEADERS`              | `X-Auth-Token x-rapidapi-key` | Headers checked for API token                        |
| `PORT`                      | `3000`                        | HTTP port to bind                                    |
| `REDIS_URL`                 | Blank                         | Redis connection string (uses memory queue if empty) |
| `EXECUTOR_TYPE`             | `auto`                        | `auto`, `isolate`, `docker`, or `process`            |
| `WORKER_CONCURRENCY`        | `4`                           | Parallel execution goroutines                        |
| `DEFAULT_CPU_TIME_LIMIT`    | `5.0`                         | Default CPU time limit in seconds                    |
| `MAX_CPU_TIME_LIMIT`        | `15.0`                        | Maximum allowed CPU time limit                       |
| `DEFAULT_WALL_TIME_LIMIT`   | `10.0`                        | Default wall-clock timeout in seconds                |
| `MAX_WALL_TIME_LIMIT`       | `30.0`                        | Maximum allowed wall-clock timeout                   |
| `DEFAULT_MEMORY_LIMIT`      | `128000`                      | Default memory limit in KB                           |
| `MAX_MEMORY_LIMIT`          | `512000`                      | Maximum allowed memory limit in KB                   |
| `MAX_PROCESSES`             | `60`                          | Process/thread limit                                 |
| `RESULT_CACHE_TTL`          | `3600`                        | Redis result expiration in seconds                   |
| `RATE_LIMIT_MAX_REQUESTS`   | `200`                         | Maximum requests per IP window                       |
| `RATE_LIMIT_WINDOW_SECONDS` | `60`                          | Rate limit window in seconds                         |

---

## Security

- **Multi-Engine Sandboxing** - Isolated namespaces, cgroups, or container boundaries.
- **Network Isolation** - Code runs with network disabled (`NetworkMode: none`).
- **Static Pre-Scan** - Pre-execution rejection of forbidden system calls (`ptrace`, `socket`, fork bombs) and sensitive paths (`/etc/shadow`, `/proc/self/`, `/var/run/docker.sock`).
- **SSRF Prevention** - Webhook callback URLs are validated against private subnets, loopbacks, and cloud metadata addresses (`169.254.169.254`).
- **Zip Slip Defense** - Multi-file ZIP extraction checks path containment and limits total uncompressed size to 20MB.
- **Non-Root Execution** - Code runs as unprivileged user `runner` (UID 1001).

---

## Makefile Quick Reference

| Command             | Action                                     |
| :------------------ | :----------------------------------------- |
| `make help`         | Display available targets                  |
| `make build`        | Compile the static binary into `./bin/cee` |
| `make install`      | Install `cee` binary to `/usr/local/bin`   |
| `make test`         | Run test suite                             |
| `make test-race`    | Run test suite with Go data race detector  |
| `make fmt`          | Format code using `gofmt`                  |
| `make vet`          | Static analysis with `go vet`              |
| `make diag`         | Run self-diagnostic suite                  |
| `make server`       | Start local API server on port 3000        |
| `make worker`       | Start standalone worker                    |
| `make docker-build` | Build language runner images               |
| `make docker-up`    | Start local Docker Compose services        |
| `make docker-down`  | Stop local Docker Compose services         |
| `make clean`        | Clean build artifacts                      |

---

## Testing

Run the automated test suite with race-condition detection:

```bash
make test-race
# or: go test -v -race ./tests/...
```

All 16 test suites verify:

- Synchronous wakeup latency
- Batch submission processing
- Pre-execution security filter
- SSRF webhook validation
- Base64 encoding/decoding
- Expected output matching
- Rate limiter enforcement

---

## License

MIT License. See [LICENSE](LICENSE) for details.
