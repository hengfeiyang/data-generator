#!/bin/bash

echo "🚀 HTTP Client Examples"
echo "======================="

echo ""
echo "1. Simple POST request (with defaults):"
go run main.go

echo ""
echo "2. Auto-generated JSON data (8 fields):"
go run main.go -fields 8

echo ""
echo "3. Multiple records per request (3 records):"
go run main.go -records 3

echo ""
echo "4. Nginx-like log data:"
go run main.go -logs

echo ""
echo "5. Multiple log records (5 records):"
go run main.go -logs -records 5

echo ""
echo "6. Custom authentication:"
go run main.go -user "admin" -pass "secret123"

echo ""
echo "7. Multiple requests (load testing):"
go run main.go -times 3

echo ""
echo "8. Complex auto-generated data (10 fields, 2 records):"
go run main.go -fields 10 -records 2

echo ""
echo "9. Custom headers:"
go run main.go -header "X-API-Version: 2.0" \
  -header "X-Request-ID: $(date +%s)"

echo ""
echo "10. Log data with multiple requests:"
go run main.go -logs -records 3 -times 2

echo ""
echo "11. Override default URL:"
go run main.go -url "https://httpbin.org/post"

echo ""
echo "12. Complete example with all features:"
go run main.go -url "https://httpbin.org/post" \
  -user "testuser" \
  -pass "testpass" \
  -fields 6 \
  -records 2 \
  -times 3 \
  -header "X-Custom-Header: example"

echo ""
echo "✅ All examples completed!" 