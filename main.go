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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// HTTPClient represents the HTTP client configuration
type HTTPClient struct {
	URL      string
	Username string
	Password string
	Data     interface{}
	Headers  map[string]string
	Timeout  time.Duration
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
	// Hours spreads record timestamps across N past hours so each record falls
	// into a distinct O2 hour partition. When <= 1, all records use "now".
	Hours int
}

// IngestConfig describes how to build a per-request O2 ingest URL.
type IngestConfig struct {
	BaseURL      string
	Org          string
	StreamPrefix string
	StreamCount  int
}

// URLFor returns the ingest URL for the Nth request (1-based).
func (ic *IngestConfig) URLFor(requestNum int) string {
	idx := (requestNum - 1) % ic.StreamCount
	return fmt.Sprintf("%s/api/%s/%s_%d/_json", ic.BaseURL, ic.Org, ic.StreamPrefix, idx)
}

// NewHTTPClient creates a new HTTP client instance
func NewHTTPClient(url, username, password string, data interface{}, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		URL:      url,
		Username: username,
		Password: password,
		Data:     data,
		Headers:  make(map[string]string),
		Timeout:  timeout,
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

// generateLogRecord generates a single log record at the given timestamp.
func generateLogRecord(now time.Time, enableBody bool) LogRecord {
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
// at the given timestamp. The "_timestamp" field (microseconds since epoch) is
// O2's partition key — that's what drives which hour bucket the record lands in.
func generateRandomData(fieldCount int, enableBody bool, now time.Time) map[string]interface{} {
	data := make(map[string]interface{})
	// Generate log record as the base data
	log := generateLogRecord(now, enableBody)
	// Add log record to data as a string
	if logBytes, err := json.Marshal(log); err == nil {
		data["message"] = string(logBytes)
	}

	// O2 partition key (microseconds) and human-readable timestamp.
	data["_timestamp"] = now.UnixMicro()
	data["timestamp"] = now.Format(time.RFC3339)
	data["request_id"] = uuid.Must(uuid.NewV7()).String()

	// Generate additional random fields (all single values, no arrays)
	fieldNames := []string{"user_id", "session_id", "action", "resource", "category", "priority", "level", "source", "target", "metadata"}

	for i := 0; i < fieldCount-3; i++ { // -3 because we already added timestamp, request_id, and message
		fieldName := fieldNames[i%len(fieldNames)] + strconv.Itoa(i)

		// Randomly choose between string, number, and boolean (no arrays)
		fieldType := rand.Intn(3) // 0=string, 1=number, 2=boolean

		switch fieldType {
		case 0: // string
			data[fieldName] = generateRandomString(rand.Intn(32) + 5)
		case 1: // number
			data[fieldName] = rand.Intn(1000000)
		case 2: // boolean
			data[fieldName] = rand.Intn(2) == 1
		}
	}

	return data
}

// timestampForRecord returns the timestamp for the i-th record in a request.
// Records are spread deterministically across [now-Hours*1h, now], one per hour,
// so a single request to a stream can fill all hour buckets at once.
func (dg *DataGenerator) timestampForRecord(idx int) time.Time {
	now := time.Now()
	if dg.Hours <= 1 {
		return now
	}
	hourOffset := idx % dg.Hours
	// Random jitter within the hour so records aren't all on the boundary.
	jitter := time.Duration(rand.Int63n(int64(time.Hour)))
	return now.Add(-time.Duration(hourOffset) * time.Hour).Add(-jitter)
}

// GenerateData generates JSON data based on the generator configuration
func (dg *DataGenerator) GenerateData() interface{} {
	if dg.RecordsPerReq == 1 {
		return generateRandomData(dg.FieldCount, dg.EnableBody, dg.timestampForRecord(0))
	}
	// Generate multiple records, each at a different hour offset.
	records := make([]map[string]interface{}, dg.RecordsPerReq)
	for i := 0; i < dg.RecordsPerReq; i++ {
		records[i] = generateRandomData(dg.FieldCount, dg.EnableBody, dg.timestampForRecord(i))
	}
	return records
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
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
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

// RunMultiple executes the POST request multiple times with optional concurrent execution.
// When ingestCfg is non-nil, each request's URL is derived from it (stream rotation).
// Otherwise c.URL is used as a fixed target.
func (c *HTTPClient) RunMultiple(times int, threads int, generator *DataGenerator, ingestCfg *IngestConfig) {
	if ingestCfg != nil {
		fmt.Printf("Running %d requests across %d stream(s) at %s/api/%s/%s_*/_json\n",
			times, ingestCfg.StreamCount, ingestCfg.BaseURL, ingestCfg.Org, ingestCfg.StreamPrefix)
	} else {
		fmt.Printf("Running %d requests to %s\n", times, c.URL)
	}
	if threads > 1 {
		fmt.Printf("Using %d concurrent threads\n", threads)
	}
	if generator != nil && generator.Hours > 1 {
		fmt.Printf("Spreading record timestamps across %d past hours\n", generator.Hours)
	}
	fmt.Println(strings.Repeat("=", 60))

	var (
		successCount  atomic.Int64
		errorCount    atomic.Int64
		totalDuration atomic.Int64 // nanoseconds
		wg            sync.WaitGroup
	)

	// Channel to distribute work among goroutines
	workChan := make(chan int, threads*4)

	// Periodic progress ticker — replaces per-request stdout, which dominates
	// runtime at 1M+ requests.
	tickerDone := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		start := time.Now()
		var lastTotal int64
		for {
			select {
			case <-tickerDone:
				return
			case <-t.C:
				s := successCount.Load()
				e := errorCount.Load()
				total := s + e
				elapsed := time.Since(start).Seconds()
				windowRate := float64(total-lastTotal) / 2.0
				avgRate := 0.0
				if elapsed > 0 {
					avgRate = float64(total) / elapsed
				}
				pct := 0.0
				if times > 0 {
					pct = float64(total) / float64(times) * 100
				}
				fmt.Printf("[%6s] %d/%d (%.1f%%)  ok=%d err=%d  %.0f req/s (avg %.0f)\n",
					time.Since(start).Round(time.Second), total, times, pct, s, e, windowRate, avgRate)
				lastTotal = total
			}
		}
	}()

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
					Timeout:  c.Timeout,
				}
				if ingestCfg != nil {
					clientCopy.URL = ingestCfg.URLFor(requestNum)
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

				totalDuration.Add(int64(resp.Duration))
				if resp.Error != nil {
					errorCount.Add(1)
					// Only print the first few errors per worker to surface
					// problems without flooding stdout.
					if errorCount.Load() <= 5 {
						fmt.Printf("[worker %d] error on req %d (%s): %v\n",
							workerID, requestNum, clientCopy.URL, resp.Error)
					}
				} else if resp.StatusCode >= 400 {
					errorCount.Add(1)
					if errorCount.Load() <= 5 {
						fmt.Printf("[worker %d] HTTP %d on req %d (%s): %s\n",
							workerID, resp.StatusCode, requestNum, clientCopy.URL, truncate(resp.Body, 200))
					}
				} else {
					successCount.Add(1)
				}
			}
		}(i)
	}

	wallStart := time.Now()

	// Send work to the channel
	for i := 1; i <= times; i++ {
		workChan <- i
	}
	close(workChan)

	// Wait for all goroutines to complete
	wg.Wait()
	close(tickerDone)

	wallElapsed := time.Since(wallStart)
	s := successCount.Load()
	e := errorCount.Load()
	td := time.Duration(totalDuration.Load())

	// Print summary
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Summary:\n")
	fmt.Printf("   Total Requests:     %d\n", times)
	fmt.Printf("   Concurrent Threads: %d\n", threads)
	fmt.Printf("   Successful:         %d\n", s)
	fmt.Printf("   Failed:             %d\n", e)
	fmt.Printf("   Wall Time:          %s\n", wallElapsed.Round(time.Millisecond))
	if wallElapsed > 0 {
		fmt.Printf("   Throughput:         %.0f req/s\n", float64(times)/wallElapsed.Seconds())
	}
	if times > 0 {
		fmt.Printf("   Avg Request:        %s\n", (td / time.Duration(times)).Round(time.Microsecond))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func main() {
	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Command line flags
	var (
		baseURL       = flag.String("url", "http://localhost:5080", "Base URL (scheme://host:port) for the O2 ingest endpoint")
		org           = flag.String("org", "default", "O2 organization name")
		streamPrefix  = flag.String("stream-prefix", "stream", "Stream name prefix (streams will be <prefix>_0, <prefix>_1, ...)")
		streams       = flag.Int("streams", 1, "Number of distinct streams to write to")
		hours         = flag.Int("hours", 1, "Spread record timestamps across N past hours (one record per hour per request)")
		username      = flag.String("user", "root@example.com", "Username for basic auth")
		password      = flag.String("pass", "Complexpass#123", "Password for basic auth")
		times         = flag.Int("times", 1, "Number of HTTP requests to send")
		threads       = flag.Int("threads", 1, "Number of concurrent threads")
		timeout       = flag.Duration("timeout", 30*time.Second, "HTTP client timeout (e.g. 30s, 1m)")
		data          = flag.String("data", "", "JSON data to send (leave empty to auto-generate)")
		header        = flag.String("header", "", "Additional header in format 'key:value'")
		fieldCount    = flag.Int("fields", 5, "Number of fields to generate in auto-generated data")
		recordsPerReq = flag.Int("records", 1, "Number of records per request")
		enableBody    = flag.Bool("body", false, "Enable body field with random size (1KB-200KB)")
		rawURL        = flag.String("raw-url", "", "If set, post to this exact URL instead of building from -url/-org/-stream-prefix (disables stream rotation)")
	)
	flag.Parse()

	// Validate required parameters
	if *baseURL == "" && *rawURL == "" {
		fmt.Println("Error: -url is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *threads < 1 {
		fmt.Println("Error: threads must be at least 1")
		os.Exit(1)
	}
	if *streams < 1 {
		fmt.Println("Error: streams must be at least 1")
		os.Exit(1)
	}
	if *hours < 1 {
		fmt.Println("Error: hours must be at least 1")
		os.Exit(1)
	}

	var jsonData interface{}
	if *data == "" {
		jsonData = nil
		fmt.Printf("Auto-generating data for each request\n")
	} else {
		if err := json.Unmarshal([]byte(*data), &jsonData); err != nil {
			fmt.Printf("Error: Invalid JSON data: %v\n", err)
			os.Exit(1)
		}
	}

	// Decide URL strategy: raw URL (no rotation) vs O2 ingest builder.
	var ingestCfg *IngestConfig
	fixedURL := ""
	if *rawURL != "" {
		fixedURL = *rawURL
	} else {
		ingestCfg = &IngestConfig{
			BaseURL:      strings.TrimRight(*baseURL, "/"),
			Org:          *org,
			StreamPrefix: *streamPrefix,
			StreamCount:  *streams,
		}
	}

	client := NewHTTPClient(fixedURL, *username, *password, jsonData, *timeout)

	if *header != "" {
		parts := strings.SplitN(*header, ":", 2)
		if len(parts) == 2 {
			client.AddHeader(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	// Run the requests
	if *data == "" {
		generator := &DataGenerator{
			FieldCount:    *fieldCount,
			RecordsPerReq: *recordsPerReq,
			EnableBody:    *enableBody,
			Hours:         *hours,
		}
		client.RunMultiple(*times, *threads, generator, ingestCfg)
	} else {
		client.RunMultiple(*times, *threads, nil, ingestCfg)
	}
}
