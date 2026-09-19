# CEE REST API Reference

The CEE (Code Execution Engine) HTTP API is a drop-in replacement for Judge0 v1.13.0, providing ultra-low latency execution of untrusted user code with microsecond queue scheduling.

---

## Base URLs

- **Production Server**: `http://100.52.188.50`
- **Versioned API Prefix**: `/v1`
- **Default Submissions**: `http://100.52.188.50/v1/submissions`
- **Legacy Root Compatibility**: `http://100.52.188.50/submissions` (Supported for Judge0 client compatibility)

---

## Authentication Headers

CEE enforces role-based access control. All protected endpoints accept authentication tokens via either:

```http
X-Auth-Token: <your-api-key>
```

or

```http
Authorization: Bearer <your-api-key>
```

| Header            | Description              | Required Roles      |
| :---------------- | :----------------------- | :------------------ |
| `X-Auth-Token`    | API key token            | `master` or `guest` |
| `X-Metrics-Token` | Prometheus metrics token | `metrics`           |

---

## Endpoints Overview

### Execution & Submissions

| Method   | Endpoint                             | Description                                            |
| :------- | :----------------------------------- | :----------------------------------------------------- |
| `POST`   | `/v1/submissions?wait=true`          | Execute code synchronously and return completed result |
| `POST`   | `/v1/submissions?wait=false`         | Submit code asynchronously (returns token immediately) |
| `GET`    | `/v1/submissions/:token`             | Fetch execution status and output by submission token  |
| `DELETE` | `/v1/submissions/:token`             | Evict completed submission from cache                  |
| `POST`   | `/v1/submissions/batch?wait=true`    | Submit multiple submissions (up to 20)                 |
| `GET`    | `/v1/submissions/batch?tokens=a,b,c` | Fetch results for multiple tokens in batch             |

### System & Languages

| Method | Endpoint            | Description                                            |
| :----- | :------------------ | :----------------------------------------------------- |
| `GET`  | `/health`           | Public server health check and uptime                  |
| `GET`  | `/v1/about`         | Server engine, version, and architecture information   |
| `GET`  | `/v1/languages`     | List all supported language IDs and compiler versions  |
| `GET`  | `/v1/languages/:id` | Get details for a specific language ID                 |
| `GET`  | `/v1/statuses`      | List all Judge0 status codes (Accepted, TLE, CE, etc.) |
| `GET`  | `/v1/statistics`    | System queue and execution runtime statistics          |

### Authentication & Keys

| Method   | Endpoint            | Description                                                     |
| :------- | :------------------ | :-------------------------------------------------------------- |
| `POST`   | `/api/auth/login`   | Validate API key with server session                            |
| `POST`   | `/api/auth/logout`  | Invalidate active session credentials                           |
| `GET`    | `/api/capabilities` | Return authorized role and capability permissions               |
| `POST`   | `/api/keys`         | Generate new AUTH or METRICS API key                            |
| `GET`    | `/api/keys`         | List all managed API keys (Master only)                         |
| `DELETE` | `/api/keys/:id`     | Revoke a key (Lockout protection prevents revoking last master) |

### Metrics

| Method | Endpoint   | Description                                                |
| :----- | :--------- | :--------------------------------------------------------- |
| `GET`  | `/metrics` | Prometheus exposition metrics (Protected by Metrics token) |

---

## Submission Schemas

### Synchronous Execution (`POST /v1/submissions?wait=true`)

#### Request Body (JSON)

```json
{
  "language_id": 71,
  "source_code": "print('Hello from CEE!')",
  "stdin": "Optional input string",
  "expected_output": "Optional expected output string",
  "cpu_time_limit": 5.0,
  "wall_time_limit": 10.0,
  "memory_limit": 256000
}
```

#### Response Body (JSON)

```json
{
  "token": "d13540c1-3fcf-4f93-b676-46c5aefb1979",
  "status": {
    "id": 3,
    "description": "Accepted"
  },
  "stdout": "Hello from CEE!\n",
  "stderr": null,
  "compile_output": null,
  "message": null,
  "time": "0.045",
  "wall_time": "0.049",
  "memory": 11468,
  "exit_code": 0
}
```

---

## Judge0 Status Codes Reference

| ID     | Status Description      | Cause                                                                  |
| :----- | :---------------------- | :--------------------------------------------------------------------- |
| **1**  | In Queue                | Waiting in memory queue or Redis channel                               |
| **2**  | Processing              | Worker is compiling or running the process                             |
| **3**  | Accepted                | Process exited with code 0 (and matched `expected_output` if provided) |
| **4**  | Wrong Answer            | Process exited code 0, but output did not match `expected_output`      |
| **5**  | Time Limit Exceeded     | Process exceeded CPU time or wall clock timeout                        |
| **6**  | Compilation Error       | Compiler (g++, rustc, javac) returned non-zero exit code               |
| **7**  | Runtime Error (SIGSEGV) | Segmentation fault                                                     |
| **8**  | Runtime Error (SIGXFSZ) | File size limit exceeded                                               |
| **9**  | Runtime Error (SIGFPE)  | Floating point exception                                               |
| **10** | Runtime Error (SIGABRT) | Process aborted                                                        |
| **11** | Runtime Error (NZEC)    | Non-zero exit code (e.g. unhandled Python/Node exception)              |
| **12** | Runtime Error (Other)   | Other unclassified OS signal                                           |
| **13** | Internal Error          | Sandbox or runner failure                                              |
| **14** | Exec Format Error       | Binary architecture mismatch                                           |

---

## Code Examples

### cURL

```bash
curl -X POST "http://100.52.188.50/v1/submissions?wait=true" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: <your-token>" \
  -d '{
    "language_id": 71,
    "source_code": "import sys; print(f\"Python {sys.version.split()[0]}\")"
  }'
```

### Python (`requests`)

```python
import requests

url = "http://100.52.188.50/v1/submissions?wait=true"
headers = {
    "Content-Type": "application/json",
    "X-Auth-Token": "<your-token>"
}
payload = {
    "language_id": 71,
    "source_code": "print(sum([x for x in range(1, 101)]))"
}

resp = requests.post(url, json=payload, headers=headers)
print(resp.json())
```

### JavaScript / Node.js (`fetch`)

```javascript
const response = await fetch("http://100.52.188.50/v1/submissions?wait=true", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "X-Auth-Token": "<your-token>"
  },
  body: JSON.stringify({
    language_id": 74,
    source_code: "const x: number = 42; console.log(x);"
  })
});

const result = await response.json();
console.log(result);
```
