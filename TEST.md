# CEE Test & Verification Suite

This document contains comprehensive end-to-end testing scenarios to verify your remote **CEE (Code Execution Engine)** deployment on AWS EC2 (`100.52.188.50`) and local CLI functionality across all supported features and installation channels.

[![Test Suite](https://img.shields.io/badge/tests-e2e%20verification-success.svg)](TEST.md)
[![Test Scenarios](https://img.shields.io/badge/scenarios-28%20checks%20passed-success.svg)](TEST.md#summary-checklist)
[![Endpoint](https://img.shields.io/badge/endpoint-%2Fv1%2Fsubmissions-blue.svg)](TEST.md#11-set-test-environment-variables)
[![Judge0 Compatible](https://img.shields.io/badge/API-Judge0%20Compatible-blue.svg)](https://judge0.com)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

---

## 0. Installation Channels (Private Repository Testing)

CEE provides 3 public installation channels that work seamlessly **without requiring GitHub credentials or a Go compiler**, even with a 100% private core repository:

### 0.1 `curl` One-Liner Shell Installer (Fastest)

Installs the precompiled native binary directly to `/usr/local/bin/cee` in under 2 seconds:

```bash
# Production server installation:
curl -fsSL http://100.52.188.50/install.sh | bash

# Verify binary is in PATH:
which cee
cee --help
```

**Expected Verdict:**

- Detects host operating system (`darwin` / `linux`) and architecture (`arm64` / `amd64`).
- Downloads prebuilt binary from `http://100.52.188.50/download/cee-${PLATFORM}-${ARCH}`.
- Outputs `==> Successfully installed CEE CLI to /usr/local/bin/cee!` and displays CLI help.

---

### 0.2 Homebrew Formula (`brew install cee`)

Installs the precompiled binary via your public Homebrew tap (`navneetguptacse/homebrew-cee`):

```bash
# Add tap and install:
brew tap navneetguptacse/cee
brew install cee

# Verify:
cee --help
```

**Expected Verdict:**

- Downloads the exact precompiled binary with SHA256 checksum verification.
- Installs without requiring `go` to be installed on the machine.

---

### 0.3 npm Global Package (`npm install -g cee-cli`)

Installs CEE via the Node.js npm package manager:

```bash
# Global installation:
npm install -g cee-cli

# Verify runner execution:
cee --help
```

**Expected Verdict:**

- Postinstall script checks for bundled binary (Option 3) or downloads from server `http://100.52.188.50/download/...` (Option 1).
- Installs `cee` CLI globally.

---

### 0.4 Direct Binary Download Endpoint Verification

Verify that the CEE server serves precompiled binaries with HTTP range and streaming headers:

```bash
# macOS Apple Silicon:
curl -sI http://100.52.188.50/download/cee-darwin-arm64 | grep -E "HTTP|Content-Type|Content-Disposition"

# Linux x86_64:
curl -sI http://100.52.188.50/download/cee-linux-amd64 | grep -E "HTTP|Content-Type|Content-Disposition"

# Traversal attack protection (should return HTTP 404):
curl -sI "http://100.52.188.50/download/../../etc/passwd" | grep "HTTP"
```

**Expected Verdict:**

- Valid binaries return `HTTP/1.1 200 OK`, `Content-Type: application/octet-stream`, `Content-Disposition: attachment; filename="cee-..."`.
- Directory traversal requests are blocked with `HTTP/1.1 404 Not Found`.

---

## 1. Authentication & Security Non-Leakage

CEE features strict role enforcement between `MASTER` and `GUEST` credentials. To prevent credential enumeration and data leakage, invalid keys or role mismatches return generic errors that **never reveal the key owner's role**.

### 1.1 Set Test Environment Variables

```bash
export CEE_SERVER="http://100.52.188.50"
export CEE_URL="http://100.52.188.50/v1/submissions"
export MASTER_KEY="bdeca5f2c9bd00428b6b50c56f632f0baba6630a6d642107afdb871584489474"
export METRICS_KEY="bdeca5f2c9bd00428b6b50c56f632f0baba6630a6d642107afdb871584489475"
```

---

### 1.2 Log in as Master

```bash
cee auth master "$MASTER_KEY" -u "$CEE_SERVER"
```

**Expected Verdict:**

```
Successfully logged in!
──────────────────────────────────────────────────────────
Server URL:      http://100.52.188.50
Key ID:          key_bootstrap_auth_1
Prefix:          bdeca5f2c9bd00428b...
Server Status:   Verified online & active
──────────────────────────────────────────────────────────
Role Permissions:
  [ALLOWED] Normal API Execution (Run, Submit, Languages)
  [ALLOWED] Generate Guest AUTH Keys
  [ALLOWED] Generate Master AUTH Keys
  [ALLOWED] Generate Metrics Keys
  [ALLOWED] Revoke & Manage All Keys
  [OPTIONAL] Metrics Access: Not configured (use 'cee auth metrics <key>')
──────────────────────────────────────────────────────────
Active profile configured. CLI commands are authenticated.
```

---

### 1.3 Role Non-Leakage Test (Security Verification)

#### A. Attempt Guest Login with Master Key (Cross-Role Attempt)

```bash
cee auth guest "$MASTER_KEY" -u "$CEE_SERVER"
```

**Expected Verdict:**

```
Error: Invalid or unauthorized API key
```

_(Confirms that the system does **not** expose that the provided key belongs to a `MASTER` account)._

#### B. Direct HTTP API Verification

```bash
curl -s -X POST "$CEE_SERVER/api/auth/login" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $MASTER_KEY" \
  -d '{"expected_role":"guest"}' | jq .
```

**Expected Verdict:**

```json
{
  "error": "Forbidden",
  "message": "Invalid or unauthorized API key"
}
```

HTTP status `403 Forbidden` with non-leaking error message.

---

### 1.4 Generate and Log In as Guest

```bash
# 1. Log back in as Master to generate a Guest key:
cee auth master "$MASTER_KEY" -u "$CEE_SERVER"

# 2. Generate a Guest key:
GUEST_KEY_OUTPUT=$(cee token generate guest -d "Automated Test Guest Key")
echo "$GUEST_KEY_OUTPUT"

# 3. Extract key token:
GUEST_KEY=$(echo "$GUEST_KEY_OUTPUT" | grep "Token:" | awk '{print $2}')

# 4. Log in as Guest:
cee auth guest "$GUEST_KEY" -u "$CEE_SERVER"
```

**Expected Verdict:**

- Output displays `Successfully logged in!`.
- Status shows `Role Permissions:` with Guest limitations (restricted key management).

---

### 1.5 Attempt Master Login with Guest Key (Privilege Escalation Guard)

```bash
cee auth master "$GUEST_KEY" -u "$CEE_SERVER"
```

**Expected Verdict:**

```
Error: Invalid or unauthorized API key
```

---

### 1.6 Logout Verification

```bash
# Log out using auth command:
cee auth logout

# Or log out using the root alias:
cee logout
```

**Expected Verdict:**

```
Successfully logged out! Active authentication credentials removed.
```

Running `cee auth status` confirms: `No active authentication session found`.

---

## 2. Server Health, Metadata & Capabilities

Log back in as Master for remaining test scenarios:

```bash
cee auth master "$MASTER_KEY" -m "$METRICS_KEY" -u "$CEE_SERVER"
```

### 2.1 Server Health Check (Public)

```bash
# Via CLI:
cee health

# Via HTTP curl:
curl -s "$CEE_SERVER/health" | jq .
```

**Expected Verdict:** `{"status": "healthy", "uptime": ...}` with HTTP 200.

### 2.2 System & Capabilities Discovery

```bash
# Engine info:
curl -s -H "X-Auth-Token: $MASTER_KEY" "$CEE_SERVER/v1/about" | jq .

# Authenticated credential identity & capabilities:
cee token whoami
```

**Expected Verdict:** Displays Judge0 API v1.13.0 compatibility, active permissions, and server metadata.

### 2.3 List Supported Languages

```bash
# Via CLI:
cee languages

# Via HTTP:
curl -s -H "X-Auth-Token: $MASTER_KEY" "$CEE_SERVER/v1/languages" | jq .
```

**Expected Verdict:** Formatted list of supported languages (Python 71, Rust 73, TypeScript 74, C++ 54, Go 60, Java 62, etc.).

---

## 3. Multi-Language Execution Matrix

Test remote code execution across core languages:

### 3.1 Python 3

```bash
cee submit -c '
import sys
print(f"Python: {sys.version.split()[0]} on {sys.platform}")
' -l py
```

**Expected:** Status `Accepted` (3), stdout showing Python 3 running on `linux`.

### 3.2 TypeScript

```bash
cee submit -c '
const numbers: number[] = [10, 20, 30, 40];
const total = numbers.reduce((acc, curr) => acc + curr, 0);
console.log("TypeScript sum:", total);
' -l ts
```

**Expected:** Status `Accepted` (3), stdout: `TypeScript sum: 100`.

### 3.3 JavaScript / Node.js

```bash
cee submit -c '
const os = require("os");
console.log("Node platform:", os.platform(), "Arch:", os.arch());
' -l js
```

**Expected:** Status `Accepted` (3), stdout: `Node platform: linux Arch: x64`.

### 3.4 C++ (g++)

```bash
cee submit -c '
#include <iostream>
int main() {
    std::cout << "Compiled C++ execution verified" << std::endl;
    return 0;
}
' -l cpp
```

**Expected:** Status `Accepted` (3), stdout: `Compiled C++ execution verified`.

### 3.5 Rust (rustc)

```bash
cee submit -c '
fn main() {
    println!("Fast compiled Rust on Linux");
}
' -l rust
```

**Expected:** Status `Accepted` (3), stdout: `Fast compiled Rust on Linux`.

### 3.6 Golang

```bash
cee submit -c '
package main
import "fmt"
func main() {
    fmt.Println("CEE Go worker execution verified")
}
' -l go
```

**Expected:** Status `Accepted` (3), stdout: `CEE Go worker execution verified`.

### 3.7 Java (OpenJDK 17)

```bash
cee submit -c '
public class Main {
    public static void main(String[] args) {
        System.out.println("Java 17 execution verified");
    }
}
' -l java
```

**Expected:** Status `Accepted` (3), stdout: `Java 17 execution verified`.

---

## 4. Standard Input (`-i`) & Output Validation (`-o`)

### 4.1 Single-line Input

```bash
cee submit -c '
import sys
name = sys.stdin.read().strip()
print(f"Hello, {name}!")
' -l py -i "Alex"
```

**Expected:** Status `Accepted` (3), stdout: `Hello, Alex!`.

### 4.2 Competitive Programming A + B Test with Expected Output

```bash
cee submit -c '#include <iostream>
int main() {
    int a, b;
    std::cin >> a >> b;
    std::cout << a + b;
}' -l cpp -i '15 35' -o '50'
```

**Expected:** Status `Accepted` (3), stdout: `50`.

### 4.3 Multi-line Input

```bash
cee submit -c '
import sys
lines = sys.stdin.read().strip().split("\n")
n = int(lines[0])
nums = [int(x) for x in lines[1].split()]
print(f"Received {n} items, sum is {sum(nums)}")
' -l py -i "$(printf "5\n10 20 30 40 50")" -o 'Received 5 items, sum is 150'
```

**Expected:** Status `Accepted` (3), stdout: `Received 5 items, sum is 150`.

---

## 5. Judge0 Status Edge Cases

### 5.1 Status 3: Accepted (Match)

```bash
curl -s -X POST "$CEE_URL?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $MASTER_KEY" \
  -d '{
    "language_id": 71,
    "source_code": "print(42)",
    "expected_output": "42\n"
  }' | jq '{status: .status.description, stdout: .stdout}'
```

**Expected:** `"Accepted"`.

### 5.2 Status 4: Wrong Answer (Mismatch)

```bash
curl -s -X POST "$CEE_URL?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $MASTER_KEY" \
  -d '{
    "language_id": 71,
    "source_code": "print(999)",
    "expected_output": "42\n"
  }' | jq '{status: .status.description, stdout: .stdout}'
```

**Expected:** `"Wrong Answer"`.

### 5.3 Status 5: Time Limit Exceeded (TLE)

```bash
cee submit -c 'while True: pass' -l py
```

**Expected:** Returns after ~5s with status `Time Limit Exceeded` (ID: 5), exit code `137`.

### 5.4 Status 6: Compilation Error (CE)

```bash
cee submit -c '
#include <iostream>
int main() {
    syntax_error_undefined_function();
    return 0;
}
' -l cpp
```

**Expected:** Status `Compilation Error` (ID: 6), `compile_output` displays compiler error.

### 5.5 Status 11: Runtime Error (NZEC)

```bash
cee submit -c 'x = 1 / 0' -l py
```

**Expected:** Status `Runtime Error (NZEC)` (ID: 11), `stderr` displays `ZeroDivisionError`.

---

## 6. Asynchronous Submission & Polling

```bash
# 1. Submit asynchronously:
ASYNC_RESP=$(curl -s -X POST "$CEE_URL?wait=false" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $MASTER_KEY" \
  -d '{
    "language_id": 71,
    "source_code": "import time; time.sleep(1); print(\"Async job finished\")"
  }')

TOKEN=$(echo "$ASYNC_RESP" | jq -r .token)
echo "Submission Token: $TOKEN"

# 2. Poll status:
cee status "$TOKEN"
```

**Expected:** Displays status progressing from `Processing (2)` to `Accepted (3)`.

---

## 7. API Key Management & Lockout Protection

### 7.1 Generate Master METRICS Key

```bash
cee token generate metrics -d "Monitoring Scraper"
```

**Expected:** Returns `cee_metrics_<64-hex>` key.

### 7.2 List Managed Keys

```bash
cee token list
```

**Expected:** Table listing key IDs, types (`auth`/`metrics`), roles (`master`/`guest`), prefixes, and statuses.

### 7.3 Last Master Lockout Protection

```bash
# Attempt to revoke the only bootstrap master key:
cee token revoke key_bootstrap_auth_1
```

**Expected:** Rejection with `lockout protection (HTTP 409): Cannot revoke the last active master AUTH API key`.

---

## 8. Security & Metrics Isolation

### 8.1 Unauthorized Metrics Access (Blocked)

```bash
curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" "$CEE_SERVER/metrics"
```

**Expected:** `HTTP Status: 401` or `403`.

### 8.2 Authorized Metrics Access

```bash
curl -s -H "X-Auth-Token: $METRICS_KEY" "$CEE_SERVER/metrics" | head -n 15
```

**Expected:** Prometheus exposition metrics (`cee_submissions_total`, etc.).

### 8.3 Cloud Metadata Protection (SSRF Guard)

```bash
cee submit -c '
import urllib.request
try:
    resp = urllib.request.urlopen("http://169.254.169.254/latest/meta-data/", timeout=2)
    print("VULNERABLE: Reached metadata:", resp.read())
except Exception as e:
    print("PROTECTED: Metadata access blocked/timed out:", type(e).__name__)
' -l py
```

**Expected:** `PROTECTED: Metadata access blocked/timed out`.

---

## 9. Automation & Deployment Verification

### 9.1 Cross-Platform Compilation (`make dist`)

```bash
make dist
```

**Expected:** Produces 5 static binaries in `./bin/dist/`:

- `cee-darwin-arm64`
- `cee-darwin-amd64`
- `cee-linux-amd64`
- `cee-linux-arm64`
- `cee-windows-amd64.exe`

### 9.2 Built-in Diagnostics

```bash
cee test
```

**Expected:** Runs self-diagnostic checks for execution and timeout handling; outputs `[PASS]`.

### 9.3 Codebase Modularization Compliance

Verify all files remain strictly under 500 lines:

```bash
find . -name "*.go" -not -path "./vendor/*" -exec wc -l {} + | sort -rn | head -n 10
```

**Expected:** Largest file is under 500 lines.

---

## Summary Checklist

| #   | Scenario                 | Command / Target                                     | Expected Verdict                         |
| :-- | :----------------------- | :--------------------------------------------------- | :--------------------------------------- |
| 1   | **curl Installer**       | `curl -fsSL http://100.52.188.50/install.sh \| bash` | Installs CLI to `/usr/local/bin/cee`     |
| 2   | **brew Installer**       | `brew install navneetguptacse/cee/cee`               | Precompiled binary installed             |
| 3   | **npm Installer**        | `npm install -g cee-cli`                             | Bundled/server binary configured         |
| 4   | **Binary Download**      | `GET /download/cee-darwin-arm64`                     | `HTTP 200 application/octet-stream`      |
| 5   | **Directory Traversal**  | `GET /download/../../etc/passwd`                     | `HTTP 404 Not Found`                     |
| 6   | **Master Login**         | `cee auth master <key>`                              | `Successfully logged in!`                |
| 7   | **Role Non-Leakage**     | `cee auth guest <MASTER_KEY>`                        | `Error: Invalid or unauthorized API key` |
| 8   | **Privilege Escalation** | `cee auth master <GUEST_KEY>`                        | `Error: Invalid or unauthorized API key` |
| 9   | **Logout & Alias**       | `cee auth logout` / `cee logout`                     | `Successfully logged out!`               |
| 10  | **Health Check**         | `cee health` / `GET /health`                         | `HTTP 200 {"status":"healthy"}`          |
| 11  | **Language Registry**    | `cee languages`                                      | Formatted language table                 |
| 12  | **Python 3**             | `cee submit -c ... -l py`                            | `Accepted (3)`                           |
| 13  | **TypeScript / Node**    | `cee submit -c ... -l ts`                            | `Accepted (3)`                           |
| 14  | **C++ / GCC**            | `cee submit -c ... -l cpp`                           | `Accepted (3)`                           |
| 15  | **Rust**                 | `cee submit -c ... -l rust`                          | `Accepted (3)`                           |
| 16  | **Go**                   | `cee submit -c ... -l go`                            | `Accepted (3)`                           |
| 17  | **Java 17**              | `cee submit -c ... -l java`                          | `Accepted (3)`                           |
| 18  | **Input & Expected**     | `-i '15 35' -o '50'`                                 | `Accepted (3)` vs `Wrong Answer (4)`     |
| 19  | **Timeout (TLE)**        | `while True: pass`                                   | `Time Limit Exceeded (5)` exit 137       |
| 20  | **Compilation Error**    | Invalid C++ syntax                                   | `Compilation Error (6)`                  |
| 21  | **Runtime Error**        | `1 / 0` in Python                                    | `Runtime Error (11)` NZEC                |
| 22  | **Async Polling**        | `cee status <token>`                                 | `Processing (2)` -> `Accepted (3)`       |
| 23  | **Guest Generation**     | `cee token generate guest`                           | Outputs new `cee_live_...` key           |
| 24  | **Lockout Guard**        | Revoke sole master key                               | `HTTP 409 Conflict`                      |
| 25  | **Metrics Security**     | `/metrics`                                           | Protected by metrics token               |
| 26  | **SSRF Cloud Guard**     | `169.254.169.254`                                    | Metadata access blocked/timed out        |
| 27  | **Cross-Platform**       | `make dist`                                          | All 5 static binaries generated          |
| 28  | **Diagnostics**          | `cee test`                                           | Core checks pass                         |
