# perfkit

A performance data collector and viewer. Capture, store, and compare Go pprof profiles and k6 load test results.

## Features

- **Collect profiles** from any Go application with pprof enabled
- **k6 load test results** - Store and compare k6 test runs
- **Web UI** for browsing and comparing profiles
- **Session grouping** to organize related profiles
- **Profile comparison** with delta visualization
- **SQLite storage** - no external dependencies

## Installation

```bash
go install github.com/flaticols/perfkit/cmd/perfkit@latest
```

Or build from source:

```bash
go build -o perfkit ./cmd/perfkit
```

## Quick Start

### 1. Start the server

```bash
perfkit server
```

Server runs on `http://localhost:8080` by default.

### 2. Capture profiles

From a running Go app with pprof enabled:

```bash
perfkit capture http://localhost:6060 --session my-test
```

Or manually via curl:

```bash
curl -s http://localhost:6060/debug/pprof/heap | \
  curl -X POST "http://localhost:8080/api/pprof/ingest?type=heap&session=my-test" --data-binary @-
```

### 3. View in browser

Open `http://localhost:8080` to browse profiles, compare metrics, and analyze performance.

## Commands

### `perfkit server`

Start the collector server and web UI.

```bash
perfkit server [OPTIONS]

Options:
  -H, --host     Server host (default: localhost)
  -p, --port     Server port (default: 8080)
      --pprof    Enable pprof endpoints for self-profiling
```

### `perfkit capture`

Capture profiles from a pprof endpoint and send to perfkit server.

```bash
perfkit capture [OPTIONS] <target>

Arguments:
  target         Target pprof URL (e.g., http://localhost:6060)

Options:
  -p, --profiles      Comma-separated profiles to capture (default: all)
                      Available: cpu,heap,goroutine,block,mutex,allocs,threadcreate
  -i, --interval      Capture interval for periodic mode (e.g., 30s, 1m)
  -s, --session       Session name for grouping profiles
      --project       Project name
      --server        Perfkit server URL (default: http://localhost:8080)
      --cpu-duration  CPU profile duration (default: 30s)
  -n, --count         Number of captures in interval mode (0=infinite)
```

**Examples:**

```bash
# Capture all profiles once
perfkit capture http://localhost:6060

# Capture specific profiles
perfkit capture http://localhost:6060 --profiles heap,goroutine,cpu

# Periodic capture every 30 seconds
perfkit capture http://localhost:6060 --interval 30s --session load-test

# Capture with custom CPU duration
perfkit capture http://localhost:6060 --cpu-duration 10s

# Send to different server
perfkit capture http://localhost:6060 --server http://perfkit.prod:8080
```

### `perfkit session`

Manage and browse sessions.

```bash
# List all sessions
perfkit session ls

# List profiles in a specific session
perfkit session profiles <session-name>
```

**Examples:**

```bash
# List all sessions
perfkit session ls
# Output:
# load-test
# monitoring
# my-session

# List profiles in a session
perfkit session profiles load-test
# Output:
# abc123  heap      2026-01-04 22:38:25  heap-profile
# def456  cpu       2026-01-04 22:38:30  cpu-profile
```

### `perfkit get`

Retrieve a specific profile from a session.

```bash
perfkit get [OPTIONS] <session> <profile_id>

Arguments:
  session        Session name
  profile_id     Profile ID

Options:
      --raw      Return raw profile data (binary)
```

**Examples:**

```bash
# Get profile metadata as JSON
perfkit get my-session abc123

# Get raw profile data and save to file
perfkit get my-session abc123 --raw > profile.pb.gz
```

## Profile Types

### Go pprof Profiles

| Type | Description | Behavior |
|------|-------------|----------|
| cpu | CPU usage sampling | Sampled over duration (default 30s) |
| heap | Memory allocations | Snapshot of current state |
| goroutine | Goroutine stacks | Snapshot of current state |
| block | Blocking operations | Cumulative since start |
| mutex | Mutex contention | Cumulative since start |
| allocs | All allocations | Cumulative since start |
| threadcreate | Thread creation | Snapshot |

### k6 Load Test Results

| Type | Description | Metrics |
|------|-------------|---------|
| k6 | Load test results | P50, P95, P99, RPS, Error Rate, Total Requests |

## k6 Integration

perfkit can store and compare k6 load test results.

### Running k6 with perfkit

```bash
# Run k6 test and export summary
k6 run --summary-export=summary.json script.js

# Ingest into perfkit
curl -X POST "http://localhost:8080/api/k6/ingest?session=load-test&name=baseline" \
  -H "Content-Type: application/json" \
  --data-binary @summary.json
```

### Compare k6 Test Runs

Store multiple test runs with the same session name, then use the web UI to compare:
- Response time improvements (P50, P95, P99)
- Throughput changes (RPS)
- Error rate variations
- Request count differences

## API

### Ingest pprof Profile

```
POST /api/pprof/ingest
```

Query parameters:
- `type` - Profile type (required)
- `session` - Session name
- `project` - Project name
- `source` - Source identifier
- `name` - Profile name
- `tag` - Tags (can be repeated)
- `cumulative` - Mark as cumulative profile (true/false)

Body: Raw pprof data (gzipped or plain)

### Ingest k6 Summary

```
POST /api/k6/ingest
```

Query parameters:
- `session` - Session name
- `project` - Project name
- `source` - Source identifier
- `name` - Profile name
- `tag` - Tags (can be repeated)

Body: k6 summary JSON (from `--summary-export`)

### List Profiles

```
GET /api/profiles?limit=50&offset=0&type=heap&project=myapp
```

### Get Profile

```
GET /api/profiles/{id}
GET /api/profiles/{id}?raw=true  # Download raw pprof data
```

### Compare Profiles

```
GET /api/profiles/compare?ids=id1,id2,id3
```

## Configuration

Create `.perfkit.yaml` in the working directory:

```yaml
data_dir: .perfkit
project: myapp
server:
  host: localhost
  port: 8080
default_tags:
  - production
```

## Enabling pprof in Your App

Add to your Go application:

```go
import _ "net/http/pprof"

func main() {
    go func() {
        http.ListenAndServe("localhost:6060", nil)
    }()
    // ... rest of your app
}
```

## Screenshots

<img width="719" height="581" alt="image" src="https://github.com/user-attachments/assets/fdba47b8-82d9-4dff-a2ed-00756a931a3a" />

<img width="719" height="581" alt="image" src="https://github.com/user-attachments/assets/530a9c84-47e4-4c41-af47-79dc1b63602e" />

<img width="719" height="581" alt="image" src="https://github.com/user-attachments/assets/a7f3f4e7-4f6b-4d74-8ea7-a1241eef9e37" />

## License

MIT
