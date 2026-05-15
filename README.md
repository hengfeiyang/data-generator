# Data generator

HTTP Client with JSON POST and Basic Auth

A Go HTTP client that can post JSON data with HTTP basic authentication, print execution time, support running multiple times, and **auto-generate JSON data** with configurable fields and records.

## Features

- ✅ POST JSON data to any URL
- ✅ HTTP Basic Authentication support
- ✅ Execution time measurement
- ✅ Multiple execution support
- ✅ **Multi-threading support** for concurrent requests
- ✅ Custom headers support
- ✅ Configurable HTTP client timeout
- ✅ **Auto-generate JSON data** with configurable fields
- ✅ **Generate multiple records per request**
- ✅ **Generate nginx-like log data**
- ✅ **Generate random body field with configurable size (1KB-500KB)**
- ✅ **OpenObserve-aware**: rotate across N streams and spread `_timestamp` over N past hours so writes land in many `(stream, hour)` partitions
- ✅ Error handling and summary statistics

## Usage

### Basic Usage

```bash
# Simple POST request (uses defaults: localhost:5080, root@example.com, Complexpass#123)
go run main.go

# POST with auto-generated JSON data (5 fields, 1 record)
go run main.go

# POST with custom auto-generated data (10 fields, 3 records per request)
go run main.go -fields 10 -records 3

# POST with custom URL but default auth
go run main.go -raw-url "https://httpbin.org/post"

# POST with custom auth
go run main.go -user "username" -pass "password"

# Run multiple times
go run main.go -times 5

# Run with multiple threads for faster execution
go run main.go -times 10 -threads 5

# Custom timeout (e.g. 1 minute for slow endpoints)
go run main.go -raw-url "https://httpbin.org/post" -timeout 1m

# Complete example with auto-generated data and multi-threading
go run main.go -raw-url "https://httpbin.org/post" \
  -user "myuser" \
  -pass "mypassword" \
  -fields 8 \
  -records 2 \
  -times 10 \
  -threads 3 \
  -header "X-Custom-Header: myvalue"

# Enable body field with random size (1KB-500KB)
go run main.go -body

# Complete example with body field enabled and multi-threading
go run main.go -raw-url "https://httpbin.org/post" \
  -fields 10 \
  -records 3 \
  -body \
  -times 10 \
  -threads 4
```

### Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-url` | Base URL (`scheme://host:port`) of the O2 ingest endpoint. The tool appends `/api/<org>/<stream>_<i>/_json` per request. | `http://localhost:5080` |
| `-org` | O2 organization name | `default` |
| `-stream-prefix` | Stream name prefix; streams become `<prefix>_0, <prefix>_1, …` | `stream` |
| `-streams` | Number of distinct streams to rotate across | `1` |
| `-hours` | Spread record timestamps across N past hours (one record per hour per request) | `1` |
| `-raw-url` | If set, POST to this exact URL instead of building from `-url/-org/-stream-prefix`. Disables stream rotation — useful for non-O2 targets like httpbin. | `""` |
| `-user` | Username for basic auth | `root@example.com` |
| `-pass` | Password for basic auth | `Complexpass#123` |
| `-times` | Number of HTTP requests to send | `1` |
| `-threads` | Number of concurrent threads | `1` |
| `-timeout` | HTTP client timeout (e.g. 30s, 1m) | `30s` |
| `-data` | JSON data to send (leave empty to auto-generate) | `""` |
| `-header` | Additional header in format `key:value` | `""` |
| `-fields` | Number of fields to generate in auto-generated data | `5` |
| `-records` | Number of records per request | `1` |
| `-body` | Enable body field with random size (1KB-500KB) | `false` |

## OpenObserve: generating many files

OpenObserve partitions ingested data by **stream × hour** — each unique
`(stream, hour)` bucket flushes to its own parquet file. The number of files on
disk is therefore:

```
files ≈ streams × hours
```

This tool ships with two flags that map directly onto those dimensions:

- `-streams N` — rotates the target URL across `N` streams (`stream_0` … `stream_{N-1}`)
- `-hours H` — sets each record's `_timestamp` (microseconds, O2's partition key)
  to a different past hour. Record `j` of a request gets
  `now − (j mod H) hours + random jitter within that hour`.

So a single request with `-records H` fills all `H` hour buckets for one stream
in one shot. To fill all `N × H` buckets, run `-times N -records H`.

### Recipe: 1,000,000 files locally

```bash
go run main.go \
  -url http://localhost:5080 \
  -org default \
  -streams 1000 \
  -hours 1000 \
  -times 1000 \
  -records 1000 \
  -threads 32
```

1,000 requests × 1,000 records = 1M log entries, spread across
1,000 streams × 1,000 hours = **1M unique `(stream, hour)` files**.

### Before you run

- **Allow backdated ingest.** Set `ZO_INGEST_ALLOWED_UPTO` larger than your
  `-hours` value (e.g. for `-hours 1000`, allow ≥ ~42 days). Otherwise O2
  silently rejects old timestamps and you'll see 4xx errors on the first batch.
- **Pause compaction while measuring.** Set `ZO_COMPACT_ENABLED=false`, or
  compaction will merge files mid-run and shrink your count.
- **Verify file count:**
  ```bash
  find <ZO_DATA_DIR>/files -name '*.parquet' | wc -l
  ```

### Tuning the shape

Pick whichever `streams × hours` product equals your target file count:

| Streams | Hours back | Files  | Time span    | Stresses                  |
|---------|------------|--------|--------------|---------------------------|
| 100     | 10,000     | 1M     | ~14 months   | time-partition listing    |
| 1,000   | 1,000      | 1M     | ~42 days     | balanced (recommended)    |
| 10,000  | 100        | 1M     | ~4 days      | stream/schema metadata    |

## Examples

#### 1. Simple POST Request (with defaults)
```bash
go run main.go
```

#### 2. Auto-generated JSON Data
```bash
# Generate 8 fields of random data
go run main.go -fields 8

# Generate 3 records per request
go run main.go -records 3

# Generate 10 fields with 2 records per request
go run main.go -fields 10 -records 2
```

#### 3. POST with Basic Authentication (custom credentials)
```bash
go run main.go -user "customuser" -pass "custompass"
```

#### 4. Load Testing (Multiple Requests)
```bash
# Sequential execution
go run main.go -data '{"test": "data"}' -times 10

# Concurrent execution with 5 threads
go run main.go -data '{"test": "data"}' -times 10 -threads 5
```

#### 5. Custom Headers
```bash
go run main.go -data '{"api_version": "v2"}' \
  -header "X-API-Version: 2.0" \
  -header "X-Request-ID: 12345"
```

#### 6. Post to a non-OpenObserve target
The default `-url` is a base host that the tool extends with `/api/<org>/<stream>/_json`.
To POST to an arbitrary URL (e.g. httpbin) without that path, use `-raw-url`:
```bash
go run main.go -raw-url "https://httpbin.org/post" -data '{"message": "Hello"}'
```

#### 7. Configurable Timeout
```bash
# Use 1 minute timeout for slow endpoints
go run main.go -raw-url "https://httpbin.org/delay/45" -timeout 1m

# Use 5 seconds for quick responses
go run main.go -raw-url "https://httpbin.org/post" -timeout 5s
```

#### 8. Enable Body Field with Random Size
```bash
# Enable body field with random size (1KB-500KB)
go run main.go -body

# Generate data with body field and multiple records
go run main.go -fields 8 -records 2 -body

# Complete example with body field and multi-threading
go run main.go -raw-url "https://httpbin.org/post" \
  -user "testuser" \
  -pass "testpass" \
  -fields 10 \
  -records 3 \
  -body \
  -times 10 \
  -threads 3
```

#### 9. Multi-Threading Examples
```bash
# High-performance load testing with 20 concurrent threads
go run main.go -raw-url "https://httpbin.org/post" -times 100 -threads 20

# Auto-generated data with concurrent execution
go run main.go -fields 15 -records 2 -times 50 -threads 10

# Stress testing with maximum concurrency
go run main.go -raw-url "https://httpbin.org/post" -times 1000 -threads 50
```

## Auto-Generated Data Types

### Standard JSON Data
The data generates random JSON data with these field types:
- **_timestamp**: Microseconds since epoch — O2's partition key (drives which hour bucket the record lands in)
- **timestamp**: Same time, RFC3339 string for human readability
- **request_id**: UUIDv7
- **message**: JSON-encoded nginx-style log record (see below)
- Plus `-fields` − 3 additional fields named `<name><i>` (e.g. `user_id0`, `session_id1`, …), each a random string, number, or boolean

Also include a `message` field that is a json struct include these fields:
- **timestamp**: Current timestamp
- **ip**: Random IP address
- **method**: HTTP method (GET, POST, PUT, DELETE, PATCH)
- **path**: Random API path
- **status**: HTTP status code (200, 201, 400, 401, 403, 404, 500)
- **bytes**: Random response size (100-10100 bytes)
- **user_agent**: Random browser user agent
- **referer**: Random referer URL
- **request_time**: Random request time (0.1-2.1 seconds)
- **remote_addr**: Random client IP
- **server_name**: Random nginx server name
- **body**: Random base64-encoded binary data (1KB-500KB) - only included when `-body` flag is enabled

## Output Format

To stay fast at high request counts, the tool does **not** print one line per
request. Instead it prints a progress tick every 2 seconds with running
counters and rates, plus the first few errors so failures aren't silent.

```
Auto-generating data for each request
Running 1000 requests across 1000 stream(s) at http://localhost:5080/api/default/stream_*/_json
Using 32 concurrent threads
Spreading record timestamps across 1000 past hours
============================================================
[    2s] 312/1000 (31.2%)  ok=312 err=0  156 req/s (avg 156)
[    4s] 641/1000 (64.1%)  ok=641 err=0  164 req/s (avg 160)
[    6s] 972/1000 (97.2%)  ok=972 err=0  165 req/s (avg 162)
============================================================
Summary:
   Total Requests:     1000
   Concurrent Threads: 32
   Successful:         1000
   Failed:             0
   Wall Time:          6.184s
   Throughput:         162 req/s
   Avg Request:        191.234ms
```

## Error Handling

The client handles various error scenarios:

- **Network errors**: Connection timeouts, DNS resolution failures
- **HTTP errors**: 4xx and 5xx status codes
- **JSON errors**: Invalid JSON data format
- **Authentication errors**: Invalid credentials

All errors are logged with detailed information and included in the final summary.

## Features in Detail

### HTTP Basic Authentication
The client automatically adds the `Authorization: Basic <base64-encoded-credentials>` header when username and password are provided.

### Auto-Generated Data
- **No data provided**: Automatically generates JSON data based on `-fields` and `-records` parameters
- **Custom data provided**: Uses the provided JSON data instead of auto-generating
- **Log data**: Generates realistic nginx-like log entries for each record
- **Body field**: When `-body` flag is enabled, includes random base64-encoded binary data (1KB-500KB) in the log record

### Timing Measurement
Each request is timed from start to finish, including:
- JSON marshaling time
- Network request time
- Response processing time

### Multiple Execution
When running multiple times:
- **Sequential execution** (default): Each request is executed one after another
- **Concurrent execution**: When using `-threads > 1`, requests are executed in parallel using goroutines
- **Thread safety**: Shared counters use `sync/atomic`; each worker holds its own per-request client copy
- **Performance**: Concurrent execution significantly reduces total execution time
- **Comprehensive statistics**: Detailed summary including thread count and timing information

### Custom Headers
You can add custom headers using the `-header` flag. The format is `key:value`.

### Multiple Records
- **Single record**: Sends one JSON object per request
- **Multiple records**: Sends an array of JSON objects per request

### Multi-Threading
The client supports concurrent execution using Go goroutines:

- **Thread Count**: Use `-threads N` to specify the number of concurrent threads (default: 1)
- **Work Distribution**: Requests are distributed evenly among available threads using a work channel
- **Thread Safety**: Each thread gets its own copy of the HTTP client to avoid race conditions
- **Synchronization**: Shared counters use `sync/atomic` (no lock contention on the hot path)
- **Performance**: Concurrent execution can significantly improve throughput for multiple requests
- **Validation**: Thread count is validated (minimum 1)

**Performance Tips:**
- Start with a small number of threads (2-5) and increase based on server capacity
- Monitor server response times and error rates when increasing thread count
- Consider network bandwidth and server limits when choosing thread count
- Use `-threads 1` for sequential execution when testing or debugging

## Building

To build the executable:

```bash
go build -o data-generator main.go
```

Then run it:

```bash
./data-generator -fields 8 -records 3
```

## Testing

You can test the client using public HTTP testing services like:
- https://httpbin.org/post
- https://httpbin.org/basic-auth/user/passwd
- https://jsonplaceholder.typicode.com/posts

## Requirements

- Go 1.24.4 or later
- Network connectivity to target URLs 
