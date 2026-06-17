# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o data-generator main.go

# Run directly
go run main.go [flags]

# Vet / format
go vet ./...
gofmt -w main.go
```

There is no test suite — `go test ./...` finds no tests. The repo is a single `main.go` plus `go.mod`/`go.sum`.

## Architecture

Single-file Go CLI (`main.go`) that POSTs JSON payloads concurrently. It started as a generic HTTP load tool and has been specialized for **OpenObserve (O2)** ingest, while still supporting arbitrary endpoints via `-raw-url`.

### Three output sinks (mutually exclusive)

- **O2 ingest builder** (default): `-url` is a base host; per request the URL is built as `<base>/api/<org>/<stream-prefix>_<i>/_json`, where `i` rotates over `-streams`. Driven by `IngestConfig.URLFor(requestNum)`.
- **Raw URL**: `-raw-url` posts to one fixed URL with no stream rotation. Use for non-O2 targets (e.g. httpbin).
- **File**: `-file PATH` writes generated records as NDJSON (one JSON record per line) to a local file and sends no HTTP, so external collectors (Vector, Fluent Bit, Filebeat, ...) can forward the data elsewhere. Stream rotation does not apply; data-generation knobs (`-hours`, `-records`, `-cardinality`, `-trace_id`, `-flat`, `-body`, `-data`) still do.

The mode is decided in `main()`: `-file` takes precedence (calls `HTTPClient.RunToFile`, `ingestCfg` unused); else if `-raw-url` is set, `ingestCfg` is `nil` and workers use `c.URL` directly; otherwise `ingestCfg` is non-nil and workers call `ingestCfg.URLFor(requestNum)` via `RunMultiple`.

### O2 partition model — the central design idea

O2 partitions ingested data by **stream × hour**. Each `(stream, hour)` bucket flushes to its own parquet file, so `files ≈ streams × hours`. The tool maps these directly:

- `-streams N` controls how many streams workers rotate across (URL dimension).
- `-hours H` controls how `_timestamp` is spread (record dimension). `DataGenerator.timestampForRecord(i)` sets record `i` to `now - (i mod H) hours + random jitter within the hour`. So **one request with `-records H` fills all H hour buckets for one stream**, and `-times N -records H` fills the full `N × H` grid.

When changing timestamp/partition logic, keep this invariant: a single request must be able to populate one full row of the `streams × hours` grid in a deterministic way.

### Concurrency model

`RunMultiple` is the hot path:
- A buffered work channel (`workChan`, size `threads*4`) distributes request numbers (1..times) to N worker goroutines.
- Each worker makes a **per-request shallow copy** of `HTTPClient` (own headers map, own URL) to avoid sharing mutable state. Don't replace this with a shared client — the URL field is rewritten per request when rotating streams.
- Counters are `atomic.Int64` (success/error/totalDuration). Avoid adding `sync.Mutex` to the hot path.
- Output is throttled: a 2s ticker prints progress, and only the first 5 errors are printed per worker, to keep stdout from dominating runtime at 1M+ requests. Preserve this behavior when adding logging.

`RunToFile` mirrors this model for `-file` output but swaps the HTTP POST for an NDJSON write: workers marshal a whole request's records into a local `bytes.Buffer`, then take a single `sync.Mutex` to append that buffer to one shared `bufio.Writer` (1 MiB buffer). The lock wraps only the write, not the marshal, so the hot path stays lock-light. Progress counts records written, not requests.

### Data generation

`DataGenerator.GenerateData()` returns either a single map (when `-records=1`) or a slice of maps. Each record contains:
- `_timestamp` (microseconds since epoch) — **this is O2's partition key**, do not rename or change the unit
- `timestamp` (RFC3339 string, same instant — human readable)
- `request_id` (UUIDv7, unique per record by default)
- `message` (JSON-encoded `LogRecord`, nginx-style)
- `trace_id` (32-char lowercase hex, W3C trace context) — only present when `-trace_id` is set
- `fields - 3` extra fields, each with a fixed type rotating string/number/bool by index (`i % 3`)

`LogRecord.Body` is a `*string` so it's omitted via `omitempty` when `-body` is off; the byte size is randomized 0–200KB inside `generateLogRecord` even though the flag advertises "1KB–500KB" (the README and code disagree — code wins).

### Cardinality model

Repeated fields (`ip`, `remote_addr`, `server_name`, `trace_id`, `user_id`, …) are bounded by `FieldCardinality` (defaults in `defaultCardinality()`, overridable via `-cardinality "field=N,..."`, `0` = unique per record). The mechanism: draw an index in `[0, N)` — power-law-skewed via `pick` so low indexes are hot, except `trace_id`/`request_id` which use `pickUniform` — then derive the value deterministically from the index by FNV hashing (`indexedHash` and friends). This keeps cardinality exact across workers and runs with no in-memory value pools and no locks; don't replace it with pre-generated pools or per-worker state. `status` is weighted (~80% 200) via `pickStatus`, not cardinality-bounded.

## Notes

- `go.mod` declares Go 1.22.2; the README mentions 1.24.4. Trust `go.mod`.
- `examples.sh` references a `-logs` flag that no longer exists in `main.go`; treat the script as outdated documentation.
