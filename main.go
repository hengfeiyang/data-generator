package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPClient represents the HTTP client configuration
type HTTPClient struct {
	URL      string
	Username string
	Password string
	Data     interface{}
	Headers  map[string]string
}

// Response represents the HTTP response
type Response struct {
	StatusCode int
	Body       string
	Duration   time.Duration
	Error      error
}

// LogRecord represents a single log entry
type LogRecord struct {
	Timestamp   string  `json:"timestamp"`
	IP          string  `json:"ip"`
	Method      string  `json:"method"`
	Path        string  `json:"path"`
	Status      int     `json:"status"`
	Bytes       int     `json:"bytes"`
	UserAgent   string  `json:"user_agent"`
	Referer     string  `json:"referer"`
	RequestTime float64 `json:"request_time"`
	RemoteAddr  string  `json:"remote_addr"`
	ServerName  string  `json:"server_name"`
	Body        *string `json:"body,omitempty"`
}

// DataGenerator handles auto-generation of JSON data
type DataGenerator struct {
	FieldCount    int
	RecordsPerReq int
	EnableBody    bool
}

// NewHTTPClient creates a new HTTP client instance
func NewHTTPClient(url, username, password string, data interface{}) *HTTPClient {
	return &HTTPClient{
		URL:      url,
		Username: username,
		Password: password,
		Data:     data,
		Headers:  make(map[string]string),
	}
}

// AddHeader adds a custom header to the request
func (c *HTTPClient) AddHeader(key, value string) {
	c.Headers[key] = value
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// generateRandomIP generates a random IP address
func generateRandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
}

// generateRandomPath generates a random URL path
func generateRandomPath() string {
	paths := []string{
		"/api/users", "/api/posts", "/api/comments", "/api/products",
		"/api/orders", "/api/categories", "/api/search", "/api/analytics",
		"/api/reports", "/api/settings", "/api/profile", "/api/dashboard",
		"/api/notifications", "/api/messages", "/api/files", "/api/upload",
	}
	return paths[rand.Intn(len(paths))]
}

// generateRandomUserAgent generates a random user agent string
func generateRandomUserAgent() string {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 14_7_1 like Mac OS X) AppleWebKit/605.1.15",
		"Mozilla/5.0 (Android 11; Mobile; rv:68.0) Gecko/68.0 Firefox/68.0",
	}
	return userAgents[rand.Intn(len(userAgents))]
}

// generateRandomReferer generates a random referer URL
func generateRandomReferer() string {
	domains := []string{
		"https://www.google.com", "https://www.facebook.com", "https://www.twitter.com",
		"https://www.linkedin.com", "https://www.github.com", "https://www.stackoverflow.com",
		"https://www.reddit.com", "https://www.youtube.com", "https://www.amazon.com",
	}
	return domains[rand.Intn(len(domains))]
}

// generateRandomBody generates random binary data and returns it as base64 encoded string
func generateRandomBody(sizeKB int) string {
	if sizeKB == 0 {
		return ""
	}

	// Convert KB to bytes
	sizeBytes := sizeKB * 1024

	// Generate random binary data
	randomData := make([]byte, sizeBytes)
	for i := range randomData {
		randomData[i] = byte(rand.Intn(256))
	}

	// Encode to base64
	return base64.StdEncoding.EncodeToString(randomData)
}

// generateLogRecord generates a single log record
func generateLogRecord(enableBody bool) LogRecord {
	now := time.Now()
	log := LogRecord{
		Timestamp:   now.Format(time.RFC3339),
		IP:          generateRandomIP(),
		Method:      []string{"GET", "POST", "PUT", "DELETE", "PATCH"}[rand.Intn(5)],
		Path:        generateRandomPath(),
		Status:      []int{200, 201, 400, 401, 403, 404, 500}[rand.Intn(7)],
		Bytes:       rand.Intn(10000) + 100,
		UserAgent:   generateRandomUserAgent(),
		Referer:     generateRandomReferer(),
		RequestTime: rand.Float64()*2.0 + 0.1, // 0.1 to 2.1 seconds
		RemoteAddr:  generateRandomIP(),
		ServerName:  "nginx-server-" + generateRandomString(4),
	}

	// Only add body field if enabled, with random size between 1KB-200KB
	if enableBody {
		// Generate random size between 1KB and 200KB
		bodySizeKB := rand.Intn(200)
		body := generateRandomBody(bodySizeKB)
		log.Body = &body
	}

	return log
}

// generateRandomData generates random JSON data with specified number of fields
func generateRandomData(fieldCount int, enableBody bool) map[string]interface{} {
	data := make(map[string]interface{})
	// Generate log record as the base data
	log := generateLogRecord(enableBody)
	// Add log record to data as a string
	if logBytes, err := json.Marshal(log); err == nil {
		data["message"] = string(logBytes)
	}

	// Always include timestamp
	data["timestamp"] = time.Now().Format(time.RFC3339)
	data["request_id"] = generateRandomString(16)

	// Generate additional random fields (all single values, no arrays)
	fieldNames := []string{"user_id", "session_id", "action", "resource", "category", "priority", "level", "source", "target", "metadata"}

	for i := 0; i < fieldCount-3; i++ { // -3 because we already added timestamp, request_id, and message
		fieldName := fieldNames[i%len(fieldNames)] + strconv.Itoa(i)

		// Randomly choose between string, number, and boolean (no arrays)
		fieldType := rand.Intn(3) // 0=string, 1=number, 2=boolean

		switch fieldType {
		case 0: // string
			data[fieldName] = generateRandomString(rand.Intn(20) + 5)
		case 1: // number
			data[fieldName] = rand.Intn(10000)
		case 2: // boolean
			data[fieldName] = rand.Intn(2) == 1
		}
	}

	return data
}

// GenerateData generates JSON data based on the generator configuration
func (dg *DataGenerator) GenerateData() interface{} {
	if dg.RecordsPerReq == 1 {
		return generateRandomData(dg.FieldCount, dg.EnableBody)
	} else {
		// Generate multiple records
		records := make([]map[string]interface{}, dg.RecordsPerReq)
		for i := 0; i < dg.RecordsPerReq; i++ {
			records[i] = generateRandomData(dg.FieldCount, dg.EnableBody)
		}
		return records
	}
}

// PostJSON sends a POST request with JSON data and basic auth
func (c *HTTPClient) PostJSON() Response {
	start := time.Now()

	// Marshal JSON data
	jsonData, err := json.Marshal(c.Data)
	if err != nil {
		return Response{
			Error:    fmt.Errorf("failed to marshal JSON: %v", err),
			Duration: time.Since(start),
		}
	}

	// Create request
	req, err := http.NewRequest("POST", c.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return Response{
			Error:    fmt.Errorf("failed to create request: %v", err),
			Duration: time.Since(start),
		}
	}

	// Set basic auth
	if c.Username != "" || c.Password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set custom headers
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return Response{
			Error:    fmt.Errorf("failed to send request: %v", err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{
			StatusCode: resp.StatusCode,
			Error:      fmt.Errorf("failed to read response body: %v", err),
			Duration:   time.Since(start),
		}
	}

	return Response{
		StatusCode: resp.StatusCode,
		Body:       string(body),
		Duration:   time.Since(start),
	}
}

// RunMultiple executes the POST request multiple times with optional concurrent execution
func (c *HTTPClient) RunMultiple(times int, threads int, generator *DataGenerator) {
	fmt.Printf("Running HTTP POST request %d times to: %s\n", times, c.URL)
	if threads > 1 {
		fmt.Printf("Using %d concurrent threads\n", threads)
	}
	fmt.Println("=" + strings.Repeat("=", 50))

	// Shared variables for thread-safe operations
	var (
		totalDuration time.Duration
		successCount  int
		errorCount    int
		mu            sync.Mutex
		wg            sync.WaitGroup
	)

	// Channel to distribute work among goroutines
	workChan := make(chan int, times)

	// Start worker goroutines
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for requestNum := range workChan {
				// Create a copy of the client for this goroutine to avoid race conditions
				clientCopy := &HTTPClient{
					URL:      c.URL,
					Username: c.Username,
					Password: c.Password,
					Headers:  make(map[string]string),
				}

				// Copy headers
				for k, v := range c.Headers {
					clientCopy.Headers[k] = v
				}

				// Generate new data for each request
				if generator != nil {
					clientCopy.Data = generator.GenerateData()
				} else {
					clientCopy.Data = c.Data
				}

				resp := clientCopy.PostJSON()

				// Thread-safe update of counters and duration
				mu.Lock()
				if resp.Error != nil {
					errorCount++
					fmt.Printf("\n[Request %d/%d] ❌ Error: %v\n", requestNum, times, resp.Error)
				} else {
					successCount++
					fmt.Printf("\n[Request %d/%d] ✅ Status: %d\n", requestNum, times, resp.StatusCode)
					fmt.Printf("📄 Response Body: %s\n", resp.Body)
				}
				fmt.Printf("⏱️  Duration: %v\n", resp.Duration)
				totalDuration += resp.Duration
				mu.Unlock()
			}
		}(i)
	}

	// Send work to the channel
	for i := 1; i <= times; i++ {
		workChan <- i
	}
	close(workChan)

	// Wait for all goroutines to complete
	wg.Wait()

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("   Total Requests: %d\n", times)
	fmt.Printf("   Concurrent Threads: %d\n", threads)
	fmt.Printf("   Successful: %d\n", successCount)
	fmt.Printf("   Failed: %d\n", errorCount)
	fmt.Printf("   Total Duration: %v\n", totalDuration)
	if times > 0 {
		fmt.Printf("   Average Duration: %v\n", totalDuration/time.Duration(times))
	}
}

func main() {
	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Command line flags
	var (
		url           = flag.String("url", "http://localhost:5080", "Target URL for POST request")
		username      = flag.String("user", "root@example.com", "Username for basic auth")
		password      = flag.String("pass", "Complexpass#123", "Password for basic auth")
		times         = flag.Int("times", 1, "Number of times to run the request")
		threads       = flag.Int("threads", 1, "Number of concurrent threads to use")
		data          = flag.String("data", "", "JSON data to send (leave empty to auto-generate)")
		header        = flag.String("header", "", "Additional header in format 'key:value' (can be used multiple times)")
		fieldCount    = flag.Int("fields", 5, "Number of fields to generate in auto-generated data")
		recordsPerReq = flag.Int("records", 1, "Number of records per request")
		enableBody    = flag.Bool("body", false, "Enable body field with random size (1KB-200KB)")
	)
	flag.Parse()

	// Validate required parameters
	if *url == "" {
		fmt.Println("❌ Error: URL is required")
		fmt.Println("Usage: go run main.go -url <target_url> [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate threads parameter
	if *threads < 1 {
		fmt.Println("❌ Error: threads must be at least 1")
		os.Exit(1)
	}
	if *threads > 100 {
		fmt.Println("⚠️  Warning: Using more than 100 threads may cause performance issues")
	}

	var jsonData interface{}

	// Check if we should auto-generate data or use provided data
	if *data == "" {
		// For auto-generated data, we'll generate it in RunMultiple
		jsonData = nil
		fmt.Printf("🔄 Will auto-generate new data for each request\n\n")
	} else {
		// Parse provided JSON data
		if err := json.Unmarshal([]byte(*data), &jsonData); err != nil {
			fmt.Printf("❌ Error: Invalid JSON data: %v\n", err)
			os.Exit(1)
		}
	}

	// Create HTTP client
	client := NewHTTPClient(*url, *username, *password, jsonData)

	// Add custom headers
	if *header != "" {
		// For simplicity, we'll just add one header
		// In a more complex implementation, you could parse multiple headers
		parts := strings.SplitN(*header, ":", 2)
		if len(parts) == 2 {
			client.AddHeader(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	// Run the requests
	if *data == "" {
		// Auto-generate data for each request
		generator := &DataGenerator{
			FieldCount:    *fieldCount,
			RecordsPerReq: *recordsPerReq,
			EnableBody:    *enableBody,
		}
		client.RunMultiple(*times, *threads, generator)
	} else {
		// Use provided data (same data for all requests)
		client.RunMultiple(*times, *threads, nil)
	}
}
