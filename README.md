# Data generator

HTTP Client with JSON POST and Basic Auth

A Go HTTP client that can post JSON data with HTTP basic authentication, print execution time, support running multiple times, and **auto-generate JSON data** with configurable fields and records.

## Features

- ✅ POST JSON data to any URL
- ✅ HTTP Basic Authentication support
- ✅ Execution time measurement
- ✅ Multiple execution support
- ✅ Custom headers support
- ✅ **Auto-generate JSON data** with configurable fields
- ✅ **Generate multiple records per request**
- ✅ **Generate nginx-like log data**
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

# Complete example with auto-generated data
go run main.go -url "https://httpbin.org/post" \
  -user "myuser" \
  -pass "mypassword" \
  -fields 8 \
  -records 2 \
  -times 3 \
  -header "X-Custom-Header: myvalue"
```

### Command Line Options

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `-url` | Target URL for POST request | `http://localhost:5080` | No |
| `-user` | Username for basic auth | `root@example.com` | No |
| `-pass` | Password for basic auth | `Complexpass#123` | No |
| `-times` | Number of times to run the request | `1` | No |
| `-data` | JSON data to send (leave empty to auto-generate) | `""` | No |
| `-header` | Additional header in format 'key:value' | `""` | No |
| `-fields` | Number of fields to generate in auto-generated data | `5` | No |
| `-records` | Number of records per request | `1` | No |

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
go run main.go -data '{"test": "data"}' -times 10
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

Running HTTP POST request 3 times to: http://localhost:5080
==================================================

[Request 1/3]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 245.123ms

[Request 2/3]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 198.456ms

[Request 3/3]
✅ Status: 200
📄 Response Body: {"status": "success", "message": "Data received"}
⏱️  Duration: 212.789ms

==================================================
📊 Summary:
   Total Requests: 3
   Successful: 3
   Failed: 0
   Total Duration: 656.368ms
   Average Duration: 218.789ms
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

### Timing Measurement
Each request is timed from start to finish, including:
- JSON marshaling time
- Network request time
- Response processing time

### Multiple Execution
When running multiple times:
- Each request is executed sequentially
- A small delay (100ms) is added between requests to be respectful to the server
- Comprehensive statistics are provided at the end

### Custom Headers
You can add custom headers using the `-header` flag. The format is `key:value`.

### Multiple Records
- **Single record**: Sends one JSON object per request
- **Multiple records**: Sends an array of JSON objects per request

## Building

To build the executable:

```bash
go build -o data_generator main.go
```

Then run it:

```bash
./data_generator -fields 8 -records 3
```

## Testing

You can test the client using public HTTP testing services like:
- https://httpbin.org/post
- https://httpbin.org/basic-auth/user/passwd
- https://jsonplaceholder.typicode.com/posts

## Requirements

- Go 1.24.4 or later
- Network connectivity to target URLs 
