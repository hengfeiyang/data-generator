package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
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
	EnableTraceID bool
	// FlatMessage merges the LogRecord fields directly into the top-level record
	// instead of JSON-encoding them into a single "message" string.
	FlatMessage bool
	// Hours spreads record timestamps across N past hours so each record falls
	// into a distinct O2 hour partition. When <= 1, all records use "now".
	Hours int
	// Cardinality bounds how many distinct values each field can take.
	Cardinality FieldCardinality
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

// generateTraceID returns a W3C trace context trace-id (32 lowercase hex chars).
func generateTraceID() string {
	var b [16]byte
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return fmt.Sprintf("%x", b[:])
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

// ---- Cardinality control ----
//
// Real-world log fields repeat: a fleet has a bounded set of client IPs, user
// ids, server names, etc., usually with a few "hot" values dominating. To
// simulate that, a bounded field draws an index in [0, cardinality) and
// derives its value deterministically from the index by hashing. No value
// pools are kept in memory and no locks are added to the worker hot path;
// cardinality N means at most N distinct values regardless of record count.

// FieldCardinality maps a field name to its max number of distinct values.
// 0 means unbounded — a fresh random value per record.
type FieldCardinality map[string]int

// defaultCardinality returns per-field defaults shaped like real-world logs:
// a small server fleet, thousands of client IPs, tens of thousands of
// users/sessions, and ~10 log lines per trace at 1M records.
func defaultCardinality() FieldCardinality {
	return FieldCardinality{
		"ip":          1000,
		"remote_addr": 1000,
		"server_name": 50,
		"request_id":  0, // unique per record, like real request ids
		"trace_id":    100000,
		"user_id":     10000,
		"session_id":  50000,
		"action":      50,
		"resource":    200,
		"category":    20,
		"priority":    5,
		"level":       6,
		"source":      100,
		"target":      500,
		"metadata":    10000,
	}
}

// parseCardinality applies "field=N,field=N" overrides from -cardinality on
// top of the defaults. N=0 restores fully-unique random values for a field.
func parseCardinality(spec string) (FieldCardinality, error) {
	fc := defaultCardinality()
	if spec == "" {
		return fc, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid cardinality entry %q (want field=N)", part)
		}
		field := strings.TrimSpace(kv[0])
		if _, known := fc[field]; !known {
			return nil, fmt.Errorf("unknown cardinality field %q (valid: %s)", field, strings.Join(fc.fieldNames(), ", "))
		}
		n, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid cardinality value in %q (want a non-negative integer)", part)
		}
		fc[field] = n
	}
	return fc, nil
}

func (fc FieldCardinality) fieldNames() []string {
	names := make([]string, 0, len(fc))
	for k := range fc {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (fc FieldCardinality) String() string {
	parts := make([]string, 0, len(fc))
	for _, k := range fc.fieldNames() {
		parts = append(parts, fmt.Sprintf("%s=%d", k, fc[k]))
	}
	return strings.Join(parts, " ")
}

// pick draws a power-law-skewed index for the field: index 0 is the hottest
// value (~18% of picks at cardinality 1000), with a long tail. ok is false
// when the field is unbounded and the caller should generate a random value.
func (fc FieldCardinality) pick(field string) (idx int, ok bool) {
	card := fc[field]
	if card <= 0 {
		return 0, false
	}
	idx = int(float64(card) * math.Pow(rand.Float64(), 4))
	if idx >= card {
		idx = card - 1
	}
	return idx, true
}

// pickUniform is pick without the hot-value skew — for trace_id and
// request_id, where real-world repeats (spans of one trace, retries of one
// request) spread evenly instead of concentrating on a few hot values.
func (fc FieldCardinality) pickUniform(field string) (idx int, ok bool) {
	card := fc[field]
	if card <= 0 {
		return 0, false
	}
	return rand.Intn(card), true
}

// indexedHash gives a stable pseudo-random 64-bit value for (field, idx) so
// the same index always derives the same field value, in any worker or run.
func indexedHash(field string, idx int) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s/%d", field, idx)
	return h.Sum64()
}

func indexedIP(field string, idx int) string {
	h := indexedHash(field, idx)
	return fmt.Sprintf("%d.%d.%d.%d", byte(h>>24), byte(h>>16), byte(h>>8), byte(h))
}

func indexedTraceID(idx int) string {
	return fmt.Sprintf("%016x%016x", indexedHash("trace_id.lo", idx), indexedHash("trace_id.hi", idx))
}

func indexedRequestID(idx int) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], indexedHash("request_id.lo", idx))
	binary.BigEndian.PutUint64(b[8:16], indexedHash("request_id.hi", idx))
	return uuid.UUID(b).String()
}

// pickStatus returns a status code weighted like real traffic: ~85% 2xx,
// mostly-benign 4xx, rare 5xx.
func pickStatus() int {
	r := rand.Intn(100)
	switch {
	case r < 80:
		return 200
	case r < 85:
		return 201
	case r < 93:
		return 404
	case r < 96:
		return 400
	case r < 98:
		return 401
	case r < 99:
		return 403
	default:
		return 500
	}
}

// generateLogRecord generates a single log record at the given timestamp.
func generateLogRecord(now time.Time, enableBody bool, card FieldCardinality) LogRecord {
	ip := generateRandomIP()
	if idx, ok := card.pick("ip"); ok {
		ip = indexedIP("ip", idx)
	}
	remoteAddr := generateRandomIP()
	if idx, ok := card.pick("remote_addr"); ok {
		remoteAddr = indexedIP("remote_addr", idx)
	}
	serverName := "nginx-server-" + generateRandomString(4)
	if idx, ok := card.pick("server_name"); ok {
		serverName = "nginx-server-" + strconv.Itoa(idx)
	}

	log := LogRecord{
		Timestamp:   now.Format(time.RFC3339),
		IP:          ip,
		Method:      []string{"GET", "POST", "PUT", "DELETE", "PATCH"}[rand.Intn(5)],
		Path:        generateRandomPath(),
		Status:      pickStatus(),
		Bytes:       rand.Intn(10000) + 100,
		UserAgent:   generateRandomUserAgent(),
		Referer:     generateRandomReferer(),
		RequestTime: rand.Float64()*2.0 + 0.1, // 0.1 to 2.1 seconds
		RemoteAddr:  remoteAddr,
		ServerName:  serverName,
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
func generateRandomData(fieldCount int, enableBody, enableTraceID, flatMessage bool, card FieldCardinality, now time.Time) map[string]interface{} {
	data := make(map[string]interface{})
	// Generate log record as the base data
	log := generateLogRecord(now, enableBody, card)
	if flatMessage {
		// Merge log record fields directly into the top-level record instead of
		// nesting them under a "message" string. Round-trip through JSON so the
		// LogRecord json tags (and omitempty for Body) are honored.
		if logBytes, err := json.Marshal(log); err == nil {
			var logMap map[string]interface{}
			if json.Unmarshal(logBytes, &logMap) == nil {
				for k, v := range logMap {
					data[k] = v
				}
			}
		}
	} else if logBytes, err := json.Marshal(log); err == nil {
		// Add log record to data as a string
		data["message"] = string(logBytes)
	}

	// O2 partition key (microseconds) and human-readable timestamp.
	data["_timestamp"] = now.UnixMicro()
	data["timestamp"] = now.Format(time.RFC3339)
	if idx, ok := card.pickUniform("request_id"); ok {
		data["request_id"] = indexedRequestID(idx)
	} else {
		data["request_id"] = uuid.Must(uuid.NewV7()).String()
	}
	if enableTraceID {
		if idx, ok := card.pickUniform("trace_id"); ok {
			data["trace_id"] = indexedTraceID(idx)
		} else {
			data["trace_id"] = generateTraceID()
		}
	}

	// Generate additional random fields (all single values, no arrays)
	fieldNames := []string{"user_id", "session_id", "action", "resource", "category", "priority", "level", "source", "target", "metadata"}

	for i := 0; i < fieldCount-3; i++ { // -3 because we already added timestamp, request_id, and message
		base := fieldNames[i%len(fieldNames)]
		fieldName := base + strconv.Itoa(i)

		// Type is fixed per field (string, number, boolean in rotation) —
		// real fields don't change type record to record, and bounded
		// cardinality only makes sense on a consistently-typed column.
		switch i % 3 {
		case 0: // string
			if idx, ok := card.pick(base); ok {
				data[fieldName] = base + "_" + strconv.Itoa(idx)
			} else {
				data[fieldName] = generateRandomString(rand.Intn(32) + 5)
			}
		case 1: // number
			if idx, ok := card.pick(base); ok {
				data[fieldName] = int(indexedHash(base, idx) % 1000000)
			} else {
				data[fieldName] = rand.Intn(1000000)
			}
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
		return generateRandomData(dg.FieldCount, dg.EnableBody, dg.EnableTraceID, dg.FlatMessage, dg.Cardinality, dg.timestampForRecord(0))
	}
	// Generate multiple records, each at a different hour offset.
	records := make([]map[string]interface{}, dg.RecordsPerReq)
	for i := 0; i < dg.RecordsPerReq; i++ {
		records[i] = generateRandomData(dg.FieldCount, dg.EnableBody, dg.EnableTraceID, dg.FlatMessage, dg.Cardinality, dg.timestampForRecord(i))
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
	if generator != nil && len(generator.Cardinality) > 0 {
		fmt.Printf("Field cardinality (0 = unique): %s\n", generator.Cardinality)
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

// appendRecords marshals a generated payload (a single record map or a slice of
// them) into buf as NDJSON — one JSON record per line. It returns the number of
// records written.
func appendRecords(buf *bytes.Buffer, data interface{}) (int, error) {
	switch v := data.(type) {
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return 0, err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		return 1, nil
	case []map[string]interface{}:
		for _, rec := range v {
			b, err := json.Marshal(rec)
			if err != nil {
				return 0, err
			}
			buf.Write(b)
			buf.WriteByte('\n')
		}
		return len(v), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return 0, err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		return 1, nil
	}
}

// RunToFile generates data and writes it as newline-delimited JSON (NDJSON) to
// path — one record per line — so log collectors (Vector, Fluent Bit, Filebeat,
// ...) can tail the file and forward records to any platform. No HTTP is sent
// and stream rotation does not apply; only the data-generation knobs (hours,
// records, cardinality, trace_id, flat, body) affect the output.
func (c *HTTPClient) RunToFile(path string, times int, threads int, generator *DataGenerator) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	// Single buffered writer guarded by a mutex. Each worker marshals a whole
	// request's records into a local buffer, then takes the lock once to flush
	// that buffer, keeping lock contention off the marshal-heavy hot path.
	w := bufio.NewWriterSize(f, 1<<20)
	var writeMu sync.Mutex

	fmt.Printf("Writing %d request(s) as NDJSON to %s\n", times, path)
	if threads > 1 {
		fmt.Printf("Using %d concurrent threads\n", threads)
	}
	if generator != nil && generator.Hours > 1 {
		fmt.Printf("Spreading record timestamps across %d past hours\n", generator.Hours)
	}
	if generator != nil && len(generator.Cardinality) > 0 {
		fmt.Printf("Field cardinality (0 = unique): %s\n", generator.Cardinality)
	}
	fmt.Println(strings.Repeat("=", 60))

	var (
		recordCount atomic.Int64
		errorCount  atomic.Int64
		wg          sync.WaitGroup
	)

	workChan := make(chan int, threads*4)

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
				recs := recordCount.Load()
				elapsed := time.Since(start).Seconds()
				windowRate := float64(recs-lastTotal) / 2.0
				avgRate := 0.0
				if elapsed > 0 {
					avgRate = float64(recs) / elapsed
				}
				fmt.Printf("[%6s] %d records  %.0f rec/s (avg %.0f)\n",
					time.Since(start).Round(time.Second), recs, windowRate, avgRate)
				lastTotal = recs
			}
		}
	}()

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			var buf bytes.Buffer
			for range workChan {
				buf.Reset()
				var payload interface{}
				if generator != nil {
					payload = generator.GenerateData()
				} else {
					payload = c.Data
				}
				n, err := appendRecords(&buf, payload)
				if err != nil {
					errorCount.Add(1)
					if errorCount.Load() <= 5 {
						fmt.Printf("[worker %d] marshal error: %v\n", workerID, err)
					}
					continue
				}
				writeMu.Lock()
				_, werr := w.Write(buf.Bytes())
				writeMu.Unlock()
				if werr != nil {
					errorCount.Add(1)
					if errorCount.Load() <= 5 {
						fmt.Printf("[worker %d] write error: %v\n", workerID, werr)
					}
					continue
				}
				recordCount.Add(int64(n))
			}
		}(i)
	}

	wallStart := time.Now()

	for i := 1; i <= times; i++ {
		workChan <- i
	}
	close(workChan)

	wg.Wait()
	close(tickerDone)

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output file: %w", err)
	}

	wallElapsed := time.Since(wallStart)
	recs := recordCount.Load()
	errs := errorCount.Load()

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Summary:\n")
	fmt.Printf("   Output File:        %s\n", path)
	fmt.Printf("   Total Requests:     %d\n", times)
	fmt.Printf("   Records Written:    %d\n", recs)
	fmt.Printf("   Failed:             %d\n", errs)
	fmt.Printf("   Wall Time:          %s\n", wallElapsed.Round(time.Millisecond))
	if wallElapsed > 0 {
		fmt.Printf("   Throughput:         %.0f rec/s\n", float64(recs)/wallElapsed.Seconds())
	}
	return nil
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
		enableTraceID = flag.Bool("trace_id", false, "Generate a trace_id (32-char hex) for each record")
		flatMessage   = flag.Bool("flat", false, "Merge log record fields into the top-level record instead of a 'message' JSON string")
		cardinality   = flag.String("cardinality", "", "Override per-field max distinct values, e.g. 'ip=500,trace_id=20000' (0 = unique per record)")
		rawURL        = flag.String("raw-url", "", "If set, post to this exact URL instead of building from -url/-org/-stream-prefix (disables stream rotation)")
		outFile       = flag.String("file", "", "If set, write generated records as NDJSON to this file instead of sending HTTP (one record per line)")
	)
	flag.Parse()

	// Validate required parameters
	if *baseURL == "" && *rawURL == "" && *outFile == "" {
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

	// Decide URL strategy: file output (no HTTP), raw URL (no rotation), or the
	// O2 ingest builder. -file takes precedence over the HTTP targets.
	var ingestCfg *IngestConfig
	fixedURL := ""
	if *outFile != "" {
		// No URL needed in file mode.
	} else if *rawURL != "" {
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

	// Build the data generator when auto-generating.
	var generator *DataGenerator
	if *data == "" {
		fieldCard, err := parseCardinality(*cardinality)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		generator = &DataGenerator{
			FieldCount:    *fieldCount,
			RecordsPerReq: *recordsPerReq,
			EnableBody:    *enableBody,
			EnableTraceID: *enableTraceID,
			FlatMessage:   *flatMessage,
			Hours:         *hours,
			Cardinality:   fieldCard,
		}
	}

	// Dispatch: write to a local file, or send HTTP requests.
	if *outFile != "" {
		if err := client.RunToFile(*outFile, *times, *threads, generator); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	client.RunMultiple(*times, *threads, generator, ingestCfg)
}
