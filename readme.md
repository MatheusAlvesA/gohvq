# Go Human Virtual Queue

A fast, easy-to-use HTTP API for managing a human waiting queue. Build one Go
binary, configure a JSON file, and let clients join, check their position, and
wait until your application admits them in FIFO order. No external database is
required.

## Highlights

- **Built for performance:** a doubly linked list and maps keep most individual
  queue operations at expected O(1) in-memory work. Queue size uses an atomic
  counter, and admission limits require no full-queue scans.
- **Simple integration:** a small JSON API for entry, polling, and administration.
- **Configurable capacity:** limit total active tickets and tickets per IP,
  or leave either limit unlimited.
- **Automatic cleanup:** remove inactive tickets and refresh positions with a
  configurable scan frequency and time budget.
- **Optional file persistence:** recover saved tickets after restarting without
  operating a separate database.
- **Deployment options:** run the binary directly or install the included systemd
  service. Native HTTPS, CORS, proxy IP headers, and optional CAPTCHA are supported.
- **Protected administration:** administrator routes use token authentication
  with constant-time comparison. Ticket keys use cryptographic randomness.

This is the queue backend. Your application provides the user interface, polls
ticket status, and decides when to finish tickets and allow people through.

## Quick start

Use the Go version declared in [go.mod](go.mod), currently Go 1.27.0, and Bash
for the helper scripts. From the project root:

```bash
./build.sh
./build/gohvq
```

The build produces `build/gohvq` with CGO disabled. By default, the API listens
on port **4242** on all IPv4 interfaces, enables persistence, and imposes no
queue capacity or per-IP limit. The generated administrator token is printed
in the startup logs.

For a local setup with a stable administrator token, create
`gohvq_config.json` in the working directory before launching:

```json
{
  "localHostOnly": true,
  "serverPort": 4242,
  "adminToken": "replace-with-your-own-long-random-secret"
}
```

Enter the queue:

```bash
curl -X POST http://localhost:4242/enter
```

The response contains a generated `key` and a zero-based `position`. Save the
returned key, then poll with it:

```bash
ticket_key='paste-the-key-returned-by-enter'
curl --get http://localhost:4242/position --data-urlencode "key=$ticket_key"
```

Polling refreshes an active ticket's ping. Poll more frequently than
`pingTimeout` to keep it active. A position of zero means the ticket is at the
front; completion is indicated by a nonzero `finishedAt` timestamp.

Finish the next ticket using the configured administrator token:

```bash
admin_token='replace-with-your-own-long-random-secret'
curl -X POST http://localhost:4242/admin/finishItems \
  -H "Authorization: $admin_token"
```

Stop the foreground process with Ctrl+C. SIGINT and SIGTERM trigger shutdown
of HTTP producers, repository cleanup, and then persistence, draining accepted
persistence actions.

## Install as a systemd service

On a Linux host using systemd, prepare your configuration and run:

```bash
./install.sh
```

The installer executes `build.sh` using the invoking user's Go environment and
uses `sudo` for system changes when needed. It creates the `gohvq` system user
and group if absent, installs the executable and unit, and reloads systemd.
It **does not start, restart, or enable** the service.

| Installed path | Purpose |
| --- | --- |
| `/usr/local/bin/gohvq` | Executable. |
| `/etc/systemd/system/gohvq.service` | systemd unit. |
| `/var/lib/gohvq` | Working directory owned by `gohvq`. |
| `/var/lib/gohvq/gohvq_config.json` | Configuration; copied from the project if present and no installed configuration exists. |
| `/var/lib/gohvq/gohvq_persistence.db` | Persistence file created at runtime when enabled. |

Existing configuration and databases are preserved. The installer does not
copy databases or TLS files. Re-running it replaces the binary and service
unit; a running process continues using its old binary until restarted.

Review the installed configuration and any TLS paths, then start the service
and optionally enable it at boot:

```bash
sudo systemctl start gohvq.service
sudo systemctl enable gohvq.service
systemctl status gohvq.service
sudo journalctl -u gohvq.service -f
```

Restart after changing configuration or installing a new binary:

```bash
sudo systemctl restart gohvq.service
```

The [unit](gohvq.service) runs as `gohvq` and grants `CAP_NET_BIND_SERVICE` so
ports such as 443 are available without running the application as root.
It restarts on failures and allows 120 seconds for graceful shutdown before
forced termination. Adjust `TimeoutStopSec` if your deployment needs longer.
After editing the installed unit, run `sudo systemctl daemon-reload` before
restarting. The unit is separate from the build.

## Configuration reference

Configuration is loaded from `gohvq_config.json` relative to the process's
**working directory**, not the executable location. Settings apply at startup;
restart to apply changes. Omitted fields keep their defaults. Missing,
unreadable, or empty files use defaults. Invalid JSON or incompatible field
types reject the whole configuration without partially applying valid fields;
the service logs the error and uses defaults.

### Server and access

| Setting | Type | Default | Purpose |
| --- | --- | --- | --- |
| `localHostOnly` | Boolean | `false` | Bind to `127.0.0.1` when true; otherwise bind to `0.0.0.0`. |
| `serverPort` | Nonnegative integer | `4242` | Listening port. Use 1–65535 for a fixed port, or 0 to let the OS select an available port. |
| `adminToken` | String | Generated random token | Required in the `Authorization` header on all administrator routes. Send the token directly, without `Bearer`. Tokens shorter than 10 characters cannot authorize requests. The generated default is logged at startup and changes on restart. |
| `clientIPHeader` | String | `""` | Header supplying the client IP, such as `X-Real-IP`. Empty uses the connection address and ignores forwarding headers. When set, entry and position requests require exactly one valid IP in this header or return 400. Administrator routes do not require it. |
| `corsAllowedOrigin` | String | `""` | Browser origin allowed through CORS, such as `https://app.example.com`. Sends that origin with `Access-Control-Allow-Credentials: true`. Empty or `*` disables these permission headers. Use a complete origin without a path or trailing slash. |
| `tlsCertFile` | String | `""` | Path to the PEM certificate chain. Set together with `tlsKeyFile` to enable HTTPS. Relative paths use the working directory. |
| `tlsKeyFile` | String | `""` | Path to the PEM private key. Must be readable by the service user and match the certificate. |
| `recaptchaSecretKey` | String | `""` | Private Google reCAPTCHA v2 secret enabling CAPTCHA on entry. Supports v2, including Invisible v2; score-based responses are rejected. Configure only one CAPTCHA provider. |
| `turnstileSecretKey` | String | `""` | Private Cloudflare Turnstile secret enabling CAPTCHA on entry. Configure only one CAPTCHA provider. |

### Queue and persistence

| Setting | Type | Default | Purpose |
| --- | --- | --- | --- |
| `maxQueueSize` | Nonnegative integer | `0` | Maximum total active tickets. Zero means unlimited. Uses the actual item count, independent of the last ticket's position. Full queues reject entry with HTTP 503. |
| `maxEntriesPerIP` | Nonnegative integer | `0` | Maximum active tickets per IP. Zero means unlimited. Reaching this limit rejects entry with HTTP 429. |
| `pingTimeout` | Nonnegative integer | `60` | Seconds without a ping before an active ticket becomes eligible for removal: elapsed time must exceed this value. Zero does not disable expiration. |
| `clearFrequency` | Nonnegative integer | `10` | Minimum seconds between the end of a cleanup scan and the next attempt. The worker checks every 500 ms. Zero makes cleanup eligible on every worker tick. |
| `clearMaxSeconds` | Nonnegative integer | `1` | Time budget in seconds per cleanup scan. Zero supplies an immediate timeout budget; it does not disable cleanup. The budget is checked between tickets and is not a hard execution deadline. |
| `persistenceEnabled` | Boolean | `true` | Recover and asynchronously save tickets in `gohvq_persistence.db` in the working directory. False keeps data only in memory for that run. |

Example with both admission limits enabled:

```json
{
  "localHostOnly": false,
  "serverPort": 4242,
  "adminToken": "replace-with-your-own-long-random-secret",
  "clientIPHeader": "",
  "corsAllowedOrigin": "https://app.example.com",
  "tlsCertFile": "",
  "tlsKeyFile": "",
  "recaptchaSecretKey": "",
  "turnstileSecretKey": "",
  "persistenceEnabled": true,
  "maxQueueSize": 10000,
  "maxEntriesPerIP": 3,
  "pingTimeout": 60,
  "clearFrequency": 10,
  "clearMaxSeconds": 1
}
```

### HTTPS and reverse proxies

Set both TLS paths to serve HTTPS exclusively on `serverPort`; use 443 if
desired. There is no separate HTTP listener or automatic HTTP-to-HTTPS redirect.
With both paths empty, the service uses HTTP. A missing partner path,
unreadable files, or an invalid certificate/key pair fails startup without
falling back to HTTP. Restart after replacing certificates.

Behind a reverse proxy, set `clientIPHeader` only when the proxy overwrites
that header with the real client address and direct access to the backend is
restricted. Without this setting, the proxy's address is used for IP limits.
Comma-separated address lists, duplicate headers, empty values, and addresses
with ports are rejected. IPv4 and equivalent IPv4-mapped IPv6 addresses share
one count. An entry's recorded IP is not changed by polling.

### CAPTCHA

Configure exactly one provider secret. Your frontend renders the provider's
widget using its public site key and sends the resulting token:

```bash
curl -X POST http://localhost:4242/enter \
  -H 'Content-Type: application/json' \
  --data '{"captchaToken":"token-from-the-widget"}'
```

Alternatively, submit `application/x-www-form-urlencoded` with one
`g-recaptcha-response` field for Google or `cf-turnstile-response` for Cloudflare.
Query-string tokens are not accepted. Bodies are limited to 16 KiB; Google
tokens to 8192 bytes and Turnstile tokens to 2048 bytes.

Missing or malformed tokens return 400; provider rejection returns 403;
verification service failures return 503. Configuring both providers also
returns 503 on entry. Other routes remain available without CAPTCHA.
Verification occurs before admission with a five-second timeout, outside queue
locks. Obtain a fresh token for every retry, including after a capacity rejection.
With neither secret configured, entry accepts an empty body and does not contact
a CAPTCHA provider.

## HTTP API

Successful responses use JSON. All `/admin/` routes require
`Authorization: <adminToken>`; missing or incorrect tokens return **403**.
Keep this token in your application's trusted backend.

| Method | Path | Successful behavior |
| --- | --- | --- |
| `POST` | `/enter` | 201 with `key` and zero-based insertion `position`. |
| `GET` | `/position?key=<key>` | 200 with `key`, `position`, and `finishedAt`. Refreshes an active ticket's ping; also finds retained finished tickets. |
| `POST` | `/admin/finishItems?n=<count>` | 200 with a list of finished tickets containing `key`, `createdAt`, and `ip`. Finishes up to the requested count from the head in FIFO order. |
| `GET` | `/admin/finishedItem/<key>` | 200 with `key`, `createdAt`, and `ip` for a retained finished ticket. |
| `DELETE` | `/admin/finishedItem/<key>` | 200 with `key`. Deletes that finished record; a valid but absent key also succeeds. |
| `DELETE` | `/admin/clearFinished` | 200 with `{"status":"ok"}`. Deletes all finished records. |
| `DELETE` | `/admin/clearQueue` | 200 with `{"status":"ok"}`. Clears active tickets, preserving finished records. |

The `n` parameter defaults to one when omitted. Explicit zero, negative, empty,
overflowing, non-integer, or duplicate values return 400 without modifying the
queue. Asking for more tickets than are available finishes only those present.

Keys contain alphanumeric characters and have the length defined by `KEY_SIZE`
in [repository.go](src/repository/repository.go); use the returned value unchanged.
Malformed keys return 400. Position lookups and finished-record reads return
404 when no matching record is found. Timestamps are Unix seconds;
`finishedAt` is zero while active and nonzero when finished. Public responses
do not expose IP addresses.

When the total queue limit is reached, entry returns **503**:

```json
{"message":"Queue is full. Please try again later."}
```

When the IP limit is reached, entry returns **429**:

```json
{"message":"Queue entry limit reached for IP"}
```

Both rejections leave queue state unchanged. Finishing tickets, removing expired
tickets during cleanup, or clearing the active queue frees capacity. Finished
tickets do not count toward admission limits and remain available until deleted
or cleared by an administrator.

## Performance and consistency

The queue favors fast in-memory operations and bounded cleanup work. Most
individual queue operations have expected O(1) in-memory cost. Batch
finalization and cleanup cost proportionally to the tickets they process;
persistence compaction also performs work proportional to stored data.
Memory grows with retained active and finished tickets. Actual request latency
also depends on locking, persistence backpressure, CAPTCHA, and runtime behavior.

FIFO ordering is preserved, but reported positions can be stale between cleanup
scans. The last ticket's position can therefore differ from the active item
count. Admission checks use the count under the same lock as insertion, so
concurrent requests cannot bypass a configured capacity limit.

Every cleanup pass starts at the head, stops when its budget expires, and waits
for the configured frequency before another attempt. A sufficiently large
active prefix can prevent later tickets from being reached across successive
scans. This is an accepted consistency/performance tradeoff. Tune cleanup
settings to the workload rather than treating expiration as an exact deadline.

Persistence is asynchronous. Add/remove actions may be dropped when the bounded
channel is full; finish/clear actions wait for channel capacity while the worker
is available. A successful HTTP response does not guarantee synchronous disk
durability. Recovery restores saved tickets even if they exceed newly configured
limits; further admissions are rejected until the applicable count falls below
the limit. The file stores keys, status, and IPs, not original timestamps or
positions; those are reconstructed on recovery.

Before moving an existing database, stop its writer and preserve a backup.
Changing `KEY_SIZE` changes both API validation and the disk format and requires
an explicit compatibility or migration plan for existing files.

## Validation and benchmarks

```bash
./test.sh
./test.sh bench
```

`test.sh` runs `go vet`, tests with the race detector, and coverage reporting to
`coverage.out`. The `bench` option additionally runs benchmarks without the race
detector. Run performance measurements without other concurrent benchmarks.

For cleanup capacity measurement and interpretation, see
[BENCHMARKS.md](src/repository/BENCHMARKS.md). Throughput depends on hardware,
memory pressure, ticket expiration patterns, and runtime behavior; cleanup-only
measurements exclude persistence and concurrent HTTP traffic and are not an
end-to-end throughput guarantee.

Further implementation details are available in the
[configuration guide](src/config/CONFIGURATION.md) and
[persistence format documentation](src/repository/PERSISTENCE.md).
