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
- ✅ **Auto-generate JSON data** with configurable fields
- ✅ **Generate multiple records per request**
- ✅ **Generate nginx-like log data**
- ✅ **Generate random body field with configurable size (1KB-500KB)**
- ✅ Detailed response logging
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
go run main.go -url "https://httpbin.org/post"

# POST with custom auth
go run main.go -user "username" -pass "password"

# Run multiple times
go run main.go -times 5

# Run with multiple threads for faster execution
go run main.go -times 10 -threads 5

# Complete example with auto-generated data and multi-threading
go run main.go -url "https://httpbin.org/post" \
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
go run main.go -url "https://httpbin.org/post" \
  -fields 10 \
  -records 3 \
  -body \
  -times 10 \
  -threads 4
```

### Command Line Options

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `-url` | Target URL for POST request | `http://localhost:5080` | No |
| `-user` | Username for basic auth | `root@example.com` | No |
| `-pass` | Password for basic auth | `Complexpass#123` | No |
| `-times` | Number of times to run the request | `1` | No |
| `-threads` | Number of concurrent threads to use | `1` | No |
| `-data` | JSON data to send (leave empty to auto-generate) | `""` | No |
| `-header` | Additional header in format 'key:value' | `""` | No |
| `-fields` | Number of fields to generate in auto-generated data | `5` | No |
| `-records` | Number of records per request | `1` | No |
| `-body` | Enable body field with random size (1KB-500KB) | `false` | No |

### Examples

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

#### 6. Override Default URL
```bash
go run main.go -url "https://httpbin.org/post" -data '{"message": "Hello"}'
```

#### 7. Enable Body Field with Random Size
```bash
# Enable body field with random size (1KB-500KB)
go run main.go -body

# Generate data with body field and multiple records
go run main.go -fields 8 -records 2 -body

# Complete example with body field and multi-threading
go run main.go -url "https://httpbin.org/post" \
  -user "testuser" \
  -pass "testpass" \
  -fields 10 \
  -records 3 \
  -body \
  -times 10 \
  -threads 3
```

#### 8. Multi-Threading Examples
```bash
# High-performance load testing with 20 concurrent threads
go run main.go -url "https://httpbin.org/post" -times 100 -threads 20

# Auto-generated data with concurrent execution
go run main.go -fields 15 -records 2 -times 50 -threads 10

# Stress testing with maximum concurrency
go run main.go -url "https://httpbin.org/post" -times 1000 -threads 50
```

## Auto-Generated Data Types

### Standard JSON Data
The data generates random JSON data with these field types:
- **timestamp**: Current timestamp in RFC3339 format
- **request_id**: Random 16-character string
- **user_id**: Random string
- **session_id**: Random string
- **action**: Random string
- **resource**: Random string
- **category**: Random string
- **priority**: Random number
- **level**: Random string
- **source**: Random string
- **target**: Random string
- **metadata**: Random array of strings

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

The client provides detailed output for each request:

```
🔄 Auto-generated data preview:
{
  "timestamp": "2024-01-15T10:30:45Z",
  "request_id": "aB3cD9eF2gH5iJ8k",
  "user_id": "user_12345",
  "session_id": "sess_67890",
  "action": "create"
}

Running HTTP POST request 5 times to: http://localhost:5080
Using 3 concurrent threads
==================================================

[Request 1/5]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 245.123ms

[Request 3/5]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 198.456ms

[Request 4/5]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 212.789ms

[Request 2/5]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 189.234ms

[Request 5/5]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 201.567ms

==================================================
📊 Summary:
   Total Requests: 5
   Concurrent Threads: 3
   Successful: 5
   Failed: 0
   Total Duration: 1.047169s
   Average Duration: 209.433ms
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
- **Thread safety**: All shared resources are protected with mutexes to prevent race conditions
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
- **Synchronization**: Shared counters and statistics are protected with mutexes
- **Performance**: Concurrent execution can significantly improve throughput for multiple requests
- **Validation**: Thread count is validated (minimum 1, warning for >100 threads)

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
