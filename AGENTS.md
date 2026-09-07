# Project guide

## Purpose and design

Go Human Virtual Queue is a Go HTTP API for a human waiting queue. Performance
is a primary requirement: most individual queue operations use expected O(1)
in-memory work through a doubly linked list and maps. Batch finalization and
cleanup have costs proportional to the items they process. Do not describe all
operations as O(1) or introduce full-queue scans into individual request paths
without considering the performance impact.

The project deliberately trades consistency for performance. Queue positions
are updated by periodic cleanup and may be stale between scans. Cleanup starts
at the head on each pass, stops when its time budget expires, and waits for the
configured frequency before trying again. A sufficiently large active prefix
can prevent later tickets from being reached across successive scans. The
maintainer explicitly accepts this behavior as part of the design. Do not
report it as a bug or introduce resumable scans solely to address it unless
requested.

## Code layout

- `main.go`: service wiring, startup, signals, and shutdown.
- `src/server`: HTTP routing, request validation, and administrator authentication.
- `src/repository/repository.go`: queue operations, locking, and periodic cleanup.
- `src/repository/structs.go`: linked-list nodes, snapshots, and persistence types.
- `src/repository/persistence.go`: asynchronous file persistence and compaction.
- `src/config`: default configuration, JSON loading, and initial service settings.
- `src/log`: console logging.

The module is `MatheusAlvesA/gohvq`. Use the Go version declared in `go.mod`
(currently 1.27.0). Production code uses `encoding/json/v2`; some tests use
`encoding/json`. Preserve that distinction unless a change requires otherwise.

## Queue and concurrency invariants

- Preserve FIFO ordering and consistency between list links and the active map.
- `mu` protects the active queue; `muFinished` protects finished items. Operations
  requiring both acquire `mu` before `muFinished`. Preserve this lock order.
- Return `ItemSnapshot` values captured under the appropriate lock. Do not expose
  mutable queue nodes or copy structs containing atomic fields as snapshots.
- `LastPing` and queue length use atomics. Keep the queue head address stable
  when clearing the queue so concurrent size queries remain safe.
- Preserve the cleanup frequency even when a scan times out.
- Cleanup statistics count completed tickets, excluding the ticket that detects
  timeout. Reuse existing survivor counts and queue-size differences instead
  of adding per-ticket instrumentation to the hot loop.

## Keys

`KEY_SIZE` in `src/repository/repository.go` is the single source of truth for
key length (currently 20 characters). Generation, validation, persistence
record sizes, test fixtures, and benchmarks must derive lengths from it.
Do not hardcode sample keys of a particular length.

Keys use `a-z`, `A-Z`, and `0-9`, generated with `crypto/rand`. The generator
also supplies default administrator tokens. Preserve cryptographic randomness
and unbiased character selection when optimizing it.

`CreateItem` retries collisions against the active map under its lock. It does
not currently check the finished map; do not claim uniqueness across both maps
or across all historical tickets.

## HTTP behavior

- Enter the queue through `POST /enter`.
- `GET /position?key=...` also refreshes an active ticket's ping.
- Finalization uses `POST /admin/finishItems`.
- With `n` absent, finalization defaults to one ticket. Explicit invalid values
  (including zero, negatives, empty values, overflow, and duplicate parameters)
  return 400 without changing queue state.
- Administrator operations require the configured authorization token. Preserve
  constant-time token comparison and tests for unauthorized mutations.

## Configuration and persistence

Configuration is read from `gohvq_config.json` relative to the working directory.
Decode into a candidate copy and apply it only on successful decoding. Invalid
JSON or field types must not partially change settings; omitted fields retain
their defaults.

Persistence uses `gohvq_persistence.db` in the working directory. Records have a
fixed size derived from `KEY_SIZE`, with a status byte, space, key, space, a 45-byte right-padded IP field, and
newline. See `src/repository/PERSISTENCE.md` for format compatibility.
Changing key length changes the disk format and API validation. Existing files
with a different key size require an explicit compatibility or migration plan;
never silently truncate keys or overwrite the developer's database.

Persistence is asynchronous and intentionally allows add/remove actions to be
dropped when its bounded channel is full. Finish and clear actions wait for
channel capacity while the worker remains available. Do not assume strict
memory/disk consistency or synchronous durability.

Compaction must preserve record offsets and the live index, and recovery must
not publish a partially validated queue. Preserve failure handling that releases
blocked producers if the persistence worker stops after a write error.

Startup order is configuration, persistence recovery, repository worker, then
HTTP. Normal shutdown stops HTTP producers and repository cleanup before
stopping persistence and draining accepted actions. Respect these lifecycle
assumptions when changing concurrency.

## Validation and benchmarks

Format modified Go files with `gofmt`. For code changes, run appropriate tests
and the standard checks:

```bash
go vet ./...
go test -race -count=1 ./...
```

`./test.sh` also generates `coverage.out` and prints coverage.
`./test.sh bench` additionally runs benchmarks. `./build.sh` builds
`build/gohvq` with CGO disabled.

HTTP lifecycle tests open local sockets; a restricted execution environment may
need permission to run them. A sandbox socket denial is not an application
regression. Persistence and configuration tests should use `t.Chdir(t.TempDir())`
to avoid touching real project data. Prefer deterministic regression tests that
assert both the response and the absence of unintended state changes.

For cleanup capacity, see `src/repository/BENCHMARKS.md`. Example:

```bash
go test ./src/repository -run '^$' -bench '^BenchmarkCleanupCapacity$' -benchtime=1x -count=3 -args -cleanup-tickets=10000000 -cleanup-seconds=1
```

Use `tickets/op` from samples where `timeouts/op=1` to measure capacity within
the budget. If the scan finishes the queue first, increase the ticket count.
Keep fixture creation outside timed work. Run performance measurements without
`-race` or concurrent benchmarks. Active and expired tickets have different
costs; these benchmarks disable persistence and concurrent HTTP traffic.
Results depend on hardware, memory pressure, and runtime behavior. Do not turn
one machine's observed throughput into a fixed test threshold or guarantee.

## Working conventions

Write repository documentation in English. Preserve unrelated local changes.
When asked for a review, distinguish actionable bugs from accepted consistency
tradeoffs, reproduce findings where practical, and avoid changing production
behavior until implementation is requested.
