# CEE User Guide

A step-by-step beginner-friendly guide to executing code, building coding platforms, and integrating with CEE.

---

## Table of Contents

1. [Quick Overview & Installation](#1-quick-overview--installation)
2. [Using CEE as a Local CLI Tool](#2-using-cee-as-a-local-cli-tool)
3. [Running CEE as a Local Server](#3-running-cee-as-a-local-server)
4. [Authenticating and Managing Keys (`cee auth` & `cee token`)](#4-authenticating-and-managing-keys-cee-auth--cee-token)
5. [Integrating CEE with Your Application](#5-integrating-cee-with-your-application)
6. [Running Multi-File Projects](#6-running-multi-file-projects)
7. [Deploying CEE to Production](#7-deploying-cee-to-production)
8. [Understanding Status Codes](#8-understanding-status-codes)
9. [Frequently Asked Questions](#9-frequently-asked-questions)

---

## 1. Quick Overview & Installation

CEE can be used in three ways:

1. **As a Command-Line Runner**: Compile and run code on your machine instantly without starting servers or databases.
2. **As an Embedded API Server**: A single binary that serves the Judge0 API for your frontend or local tests.
3. **As a Distributed Execution Cluster**: Backed by Redis and Docker for high-volume production websites (like LeetCode or HackerRank).

### Installation Options

#### Option A: One-Liner Shell Script (Fastest)

Install the precompiled static binary to `/usr/local/bin/cee` in seconds:

```bash
# From your self-hosted CEE server:
curl -fsSL http://<server-ip>/install.sh | bash

# Or directly from GitHub:
curl -fsSL https://raw.githubusercontent.com/navneetguptacse/cee.io/main/install.sh | bash
```

#### Option B: Homebrew (macOS & Linux)

```bash
brew tap navneetguptacse/cee https://github.com/navneetguptacse/cee.io
brew install navneetguptacse/cee/cee
```

#### Option C: npm Global Package

```bash
npm install -g cee-cli
```

#### Option D: Build from Source

```bash
cd cee.io
make build
sudo make install
```

### Makefile Quick Reference

| Command             | Purpose                                         |
| :------------------ | :---------------------------------------------- |
| `make help`         | Show all available targets and descriptions     |
| `make build`        | Compile the static binary into `./bin/cee`      |
| `make install`      | Install `cee` into `/usr/local/bin`             |
| `make test`         | Run all test suites                             |
| `make test-race`    | Run test suites with data race detector enabled |
| `make fmt`          | Format all source code with `gofmt`             |
| `make vet`          | Run static analysis with `go vet`               |
| `make diag`         | Run CEE built-in self-diagnostics               |
| `make server`       | Start the local CEE API server on port 3000     |
| `make worker`       | Start a standalone background worker            |
| `make docker-build` | Build all language runner images                |
| `make docker-up`    | Start local development cluster                 |
| `make docker-down`  | Stop local development cluster                  |
| `make clean`        | Remove `./bin` and temporary files              |

---

## 2. Using CEE as a Local CLI Tool

The `cee run` command lets you execute code immediately on your machine without starting background servers or databases. It supports inline code strings, piped input from `stdin`, or direct file execution with automatic language detection.

### Running Inline Code Directly

Execute code without creating a file using `-c` (or `--code`) and `-l` (or `--lang`):

```bash
# Python
cee run -c "print(100 * 5)" -l py

# JavaScript
cee run -c "console.log(21 * 2)" -l js

# Bash
cee run -c "echo $((10 + 20))" -l bash
```

Output:

```text
Executing inline (Python (3.8.10)) with process...

──────────────────── Execution Result ────────────────────
Status:    Accepted (ID: 3)
Exit Code: 0
Wall Time: 0.071s (total CLI: 71ms)

[Stdout]:
500
──────────────────────────────────────────────────────────
```

### Passing Inline Code Positionally

If you provide a string that is not an existing file on disk alongside `-l`, `cee` treats it directly as source code:

```bash
cee run "print(25 * 4)" -l python
```

### Piping Code via Standard Input (`stdin`)

You can pipe code directly from terminal pipelines:

```bash
echo "print('hello from stdin pipe')" | cee run -l py
```

### Supplying Input Data to Your Code (`--stdin`)

If your inline program expects standard input, supply the input data via the `--stdin` flag:

```bash
cee run -c "name = input(); print(f'Hello, {name}!')" -l py --stdin "Alice"
```

Output:

```text
[Stdout]:
Hello, Alice!
```

### Supported Language Aliases and IDs

The `-l` / `--lang` flag accepts language names, common short aliases, or numeric Judge0 IDs:

| Language   | Common Aliases                       | Canonical ID |
| :--------- | :----------------------------------- | :----------- |
| Python     | `py`, `python`, `python3`            | `71`         |
| JavaScript | `js`, `javascript`, `node`, `nodejs` | `63`         |
| TypeScript | `ts`, `typescript`                   | `74`         |
| Go         | `go`, `golang`                       | `60`         |
| C++        | `cpp`, `c++`, `g++`                  | `54`         |
| C          | `c`, `gcc`                           | `50`         |
| Java       | `java`, `openjdk`                    | `62`         |
| Rust       | `rs`, `rust`                         | `73`         |
| Bash       | `sh`, `bash`, `shell`                | `46`         |

### Running from Local Files

When passing a file path, `cee run` automatically detects the language from the file extension:

#### Run Python

Create a file named `hello.py`:

```python
name = input()
print(f"Hello, {name}!")
```

Run it:

```bash
cee run hello.py --stdin "Alice"
```

#### Run C++ with Automated Answer Checking

Create a file named `sum.cpp`:

```cpp
#include <iostream>
using namespace std;

int main() {
    int a, b;
    if (cin >> a >> b) {
        cout << a + b;
    }
    return 0;
}
```

Check your program against test inputs and expected outputs:

```bash
# Correct answer -> Status: Accepted (ID: 3)
cee run sum.cpp --stdin "15 25" --expected "40"

# Incorrect answer -> Status: Wrong Answer (ID: 4)
cee run sum.cpp --stdin "15 25" --expected "50"
```

#### Run Go Code

Create `main.go`:

```go
package main
import "fmt"

func main() {
    fmt.Println("CEE is running Go natively!")
}
```

Run it:

```bash
cee run main.go
```

### Useful CLI Flags for `cee run`

| Flag         | Short | Description                                 | Example             |
| :----------- | :---- | :------------------------------------------ | :------------------ |
| `--code`     | `-c`  | Inline source code string to execute        | `-c "print(100)"`   |
| `--lang`     | `-l`  | Language name, alias, or ID                 | `-l py`, `-l 71`    |
| `--stdin`    |       | Feed input data to stdin of the program     | `--stdin "100 200"` |
| `--expected` |       | Compare stdout against expected string      | `--expected "300"`  |
| `--timeout`  | `-t`  | Execution timeout in seconds (default: 5.0) | `--timeout 2.0`     |
| `--executor` | `-e`  | Sandbox engine (`process` or `docker`)      | `--executor docker` |

---

## 3. Running CEE as a Local Server

If you are developing a web application, online editor, or grading system, you can start the CEE API server with a single command:

```bash
cee server --port 3000
```

The server starts immediately using an internal, high-speed queue without requiring Docker or Redis.

### Test Server Health

```bash
curl http://localhost:3000/health
```

Expected response:

```json
{
  "status": "healthy",
  "uptime": 1.25,
  "timestamp": "2026-09-16T00:00:00Z"
}
```

### Run Code Synchronously (`?wait=true`)

When `wait=true` is provided, CEE holds the HTTP connection open and returns the completed execution result immediately:

```bash
curl -X POST "http://localhost:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -d '{
    "language_id": 71,
    "source_code": "print(100 * 5)"
  }'
```

Response:

```json
{
  "token": "78470f9b-21ed-4eea-a7e6-f9cc4adb3f60",
  "status": {
    "id": 3,
    "description": "Accepted"
  },
  "stdout": "500\n",
  "stderr": null,
  "compile_output": null,
  "time": "0.048",
  "wall_time": "0.048",
  "exit_code": 0
}
```

### Run Code Asynchronously (Standard Queue Pattern)

For high-concurrency applications:

**Step 1: Submit code**

```bash
curl -X POST "http://localhost:3000/submissions" \
  -H "Content-Type: application/json" \
  -d '{
    "language_id": 71,
    "source_code": "import time; time.sleep(1); print(\"Done\")"
  }'
```

Returns: `{"token": "4adc7ae9-6d80-4119-a9d1-d2a2432ae213"}`

**Step 2: Fetch result using the token**

```bash
curl "http://localhost:3000/submissions/4adc7ae9-6d80-4119-a9d1-d2a2432ae213"
```

---

## 4. Authenticating and Managing Keys (`cee auth` & `cee token`)

CEE incorporates role-based authentication with strict non-leakage security. Invalid keys or role mismatches return generic errors that never reveal credential ownership.

### Logging In

#### As Master (Full Administrative Access)

Master credentials grant full access to execute code, configure optional Prometheus metrics keys, and generate or revoke API keys:

```bash
cee auth master <your-master-token> -u http://<server-ip>:3000
```

#### As Guest (Execution & Guest Delegation)

Guest credentials allow code execution and generating additional guest keys:

```bash
cee auth guest <your-guest-token> -u http://<server-ip>:3000
```

### Inspecting Session & Permissions

View your active session profile, server URL, and granted capabilities:

```bash
cee auth status
# or
cee token whoami
```

### Generating New API Keys

Generate a new Guest key:

```bash
cee token generate guest -d "Frontend Runner"
```

Generate a new Master key (Master role required):

```bash
cee token generate master -d "Secondary Admin"
```

### Listing and Revoking Keys

List all active keys stored on the server:

```bash
cee token list
```

Revoke a key (protected by last-master lockout guard):

```bash
cee token revoke key_xxxx
```

### Logging Out

Clear your locally saved profile credentials:

```bash
cee logout
# or
cee auth logout
```

---

## 5. Integrating CEE with Your Application

CEE is 100% compatible with the Judge0 API standard.

### JavaScript / TypeScript Example

```javascript
async function runCode(code, languageId, input = "") {
  const response = await fetch("http://localhost:3000/submissions?wait=true", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Auth-Token": "your-secret-token", // if auth is enabled
    },
    body: JSON.stringify({
      source_code: code,
      language_id: languageId,
      stdin: input,
    }),
  });

  const result = await response.json();

  if (result.status.id === 3) {
    console.log("Success! Output:", result.stdout);
  } else if (result.status.id === 6) {
    console.error("Compilation Error:", result.compile_output);
  } else {
    console.error("Runtime Error / Status:", result.status.description);
    console.error(result.stderr);
  }
}

// Execute Python code
runCode("print('Hello from Node.js!')", 71);
```

### Python Example

```python
import requests

def execute_code(source_code: str, language_id: int, stdin: str = ""):
    url = "http://localhost:3000/submissions?wait=true"
    payload = {
        "source_code": source_code,
        "language_id": language_id,
        "stdin": stdin
    }
    headers = {"Content-Type": "application/json"}

    response = requests.post(url, json=payload, headers=headers)
    data = response.json()

    print(f"Status: {data['status']['description']}")
    print(f"Stdout: {data.get('stdout')}")
    print(f"Time:   {data.get('time')}s")

# Execute C++ code
cpp_code = """
#include <iostream>
int main() {
    std::cout << "Executed in C++!";
    return 0;
}
"""
execute_code(cpp_code, 54)
```

---

## 6. Running Multi-File Projects

For projects containing multiple source files, libraries, or custom build scripts, use **Language ID 89**.

### Package Structure

Create a directory containing your source code and a bash script named `run` (or `run.sh`):

```text
my_project/
├── main.py
├── helper.py
└── run
```

Contents of `run`:

```bash
#!/bin/bash
python3 main.py
```

Make `run` executable:

```bash
chmod +x my_project/run
```

### Create ZIP and Submit

Compress the files into a `.zip` archive:

```bash
cd my_project && zip -r ../project.zip . && cd ..
```

Encode to Base64 and send:

```bash
ZIP_B64=$(base64 < project.zip | tr -d '\n')

curl -X POST "http://localhost:3000/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -d "{
    \"language_id\": 89,
    \"additional_files\": \"$ZIP_B64\"
  }"
```

CEE unpacks the archive in a dedicated sandbox, performs path-traversal safety checks, and executes `run`.

---

## 7. Deploying CEE to Production

### Option A: Standalone Binary on a Linux Server

If you already have a server and want to run CEE as a background service:

1. Build the binary on the server or copy it over:

```bash
./scripts/build.sh
```

2. Generate a secure token and run CEE:

```bash
# Generate a random 64-character token
AUTH_TOKEN=$(openssl rand -hex 32)
echo "Your production token: $AUTH_TOKEN"

# Run CEE with authentication enabled
AUTH_TOKEN=$AUTH_TOKEN PORT=3000 ./bin/cee server &
```

Now all requests must provide the header:

```bash
-H "X-Auth-Token: <your-token>"
```

### Option B: Production Docker Compose (with Automatic SSL)

The production configuration bundles CEE with **Caddy** (automatic HTTPS from Let's Encrypt), **Redis 7**, and **Prometheus**.

1. Point your domain DNS A record to your server IP.
2. Edit `.env`:

```bash
cat > .env <<EOF
AUTH_TOKEN=$(openssl rand -hex 32)
METRICS_TOKEN=$(openssl rand -hex 16)
DOMAIN=cee.yourdomain.com
WORKER_CPUS=2.0
WORKER_MEMORY=2G
EOF
```

3. Start the stack:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

Caddy will automatically request and install an SSL certificate for `cee.yourdomain.com`.

### Option C: Prebuilt Container Image (Docker Hub & GHCR)

Run the prebuilt multi-platform container directly without compiling Go. See [HUB.md](HUB.md) for complete container documentation and options:

```bash
docker run -d -p 3000:3000 \
  -e AUTH_TOKEN="$(openssl rand -hex 32)" \
  -e MAX_WORKERS=8 \
  --name cee navneetguptacse/cee:latest
```

---

## 8. Understanding Status Codes

When CEE completes an execution, `status.id` indicates the outcome:

| Status ID | Status Name             | Meaning and Resolution                                                                                                     |
| :-------- | :---------------------- | :------------------------------------------------------------------------------------------------------------------------- |
| **3**     | Accepted                | The code executed successfully with exit code 0. If `expected_output` was provided, stdout matched.                        |
| **4**     | Wrong Answer            | The code ran without errors, but its stdout did not match `expected_output`. Check formatting or logic.                    |
| **5**     | Time Limit Exceeded     | The code ran longer than `wall_time_limit` or `cpu_time_limit`. Check for infinite loops or heavy calculations.            |
| **6**     | Compilation Error       | The compiler returned a non-zero exit code or the code analyzer rejected a prohibited system call. Check `compile_output`. |
| **7**     | Runtime Error (SIGSEGV) | Segmentation fault. Usually caused by invalid array indices or null pointer dereferences in C/C++.                         |
| **8**     | Runtime Error (SIGXFSZ) | File size limit exceeded. Program generated more output files than permitted.                                              |
| **9**     | Runtime Error (SIGFPE)  | Floating point exception, such as division by zero.                                                                        |
| **10**    | Runtime Error (SIGABRT) | Program aborted itself by calling `abort()` or failing an assertion.                                                       |
| **11**    | Runtime Error (NZEC)    | Non-zero exit code. An unhandled exception was thrown (e.g. `IndexError` in Python or uncaught error in Node.js).          |

---

## 9. Frequently Asked Questions

### What languages are supported out of the box?

Run `cee languages` to view all supported runtimes. Canonical IDs include:

- `71` / `92`: Python (3.8.10 / 3.11)
- `63` / `93` / `102`: JavaScript (Node.js 18 / 22)
- `74` / `94`: TypeScript (5.0.3)
- `50`: C (GCC 9.2)
- `54`: C++ (GCC 9.2)
- `60` / `95`: Go (1.22)
- `62`: Java (OpenJDK 17)
- `73`: Rust (1.75)
- `46`: Bash (5.0)
- `89`: Multi-file program (ZIP archive)

### Can user code access the internet?

No. Network access is disabled by default (`NetworkMode: none`). User code cannot communicate with external servers, cloud metadata services, or other local processes.

### How do I handle large code files or inputs?

Pass `base64_encoded=true` as a query parameter. When set:

- Input fields (`source_code`, `stdin`, `expected_output`) must be base64-encoded strings.
- Output fields (`stdout`, `stderr`, `compile_output`) will be returned as base64-encoded strings.
  This avoids JSON string escaping and quote syntax errors.
