# Configuration

Configuration is loaded from `gohvq_config.json` in the working directory.
Settings are applied at startup; restart the service after editing the file.
Omitted fields retain their defaults. If the file is missing, unreadable, or
empty, the service uses defaults. Invalid JSON or incompatible field types
reject the entire configuration without partially applying valid fields.

| Field | Type | Default | Behavior |
| --- | --- | --- | --- |
| `localHostOnly` | Boolean | `false` | Listen on `127.0.0.1` when `true`, or all IPv4 interfaces (`0.0.0.0`) when `false`. |
| `serverPort` | Nonnegative integer | `4242` | HTTP listening port. Use `1`–`65535` for a fixed port; `0` lets the operating system choose an available port. |
| `adminToken` | String | Cryptographically generated token | Token required in the `Authorization` header for administrator routes. Send the token directly, without a `Bearer` prefix. Tokens shorter than 10 characters cannot authorize requests. The generated token uses the repository's `KEY_SIZE` and is logged when used as the default. |
| `clientIPHeader` | String | `""` | Header containing the client IP. Empty or omitted uses the connection address and ignores request headers. When configured, `POST /enter` and `GET /position` require this header with a single valid IPv4 or IPv6 address; otherwise it returns HTTP 400 and logs the rejection before executing the handler. Administrator routes ignore this setting. |
| `corsAllowedOrigin` | String | `""` | Origin sent in `Access-Control-Allow-Origin` together with `Access-Control-Allow-Credentials: true`. Empty, omitted, or `*` disables these permission headers. Use a complete origin, including scheme and optional port, without a path or trailing slash (for example, `https://app.example.com`). |
| `persistenceEnabled` | Boolean | `true` | Recover and asynchronously persist queue data in `gohvq_persistence.db` in the working directory. When `false`, queue data exists only in memory for that run. |
| `clearMaxSeconds` | Nonnegative integer | `1` | Time budget in seconds for each cleanup scan. `0` gives an immediate timeout budget; it does not disable the budget. |
| `pingTimeout` | Nonnegative integer | `60` | An active ticket becomes eligible for removal when the seconds since its last ping exceed this value. `GET /position?key=...` refreshes the ping of an active ticket. `0` does not disable expiration. |
| `clearFrequency` | Nonnegative integer | `10` | Minimum interval in seconds between the end of a cleanup scan and the next scan. The worker checks whether cleanup is due every 500 milliseconds. `0` makes cleanup eligible on every worker tick. |
| `maxEntriesPerIP` | Nonnegative integer | `0` | Maximum active tickets per IP. `0` means unlimited. See the admission behavior below. |

For example, this sets all fields explicitly and allows three active tickets
per IP:

```json
{
  "localHostOnly": false,
  "serverPort": 4242,
  "clientIPHeader": "",
  "corsAllowedOrigin": "https://app.example.com",
  "adminToken": "replace-with-your-own-secret-token",
  "persistenceEnabled": true,
  "clearMaxSeconds": 1,
  "pingTimeout": 60,
  "clearFrequency": 10,
  "maxEntriesPerIP": 3
}
```

Cleanup removes expired active tickets and updates queue positions. Positions
can remain stale between scans. Each scan starts at the head, stops when its
budget expires, and still waits for `clearFrequency` before another attempt.
A sufficiently large active prefix can keep later tickets from being reached
across successive scans; this is an accepted performance tradeoff.

Persistence is asynchronous: add/remove actions may be dropped when its bounded
channel is full, while finish/clear actions wait for capacity while the worker
is available. An HTTP success does not imply synchronous disk durability.
See [the persistence format documentation](../repository/PERSISTENCE.md) for
record layout and compatibility.

Set `maxEntriesPerIP` to a nonnegative integer to limit the number of active
queue tickets for each IP. The default, `0`, means unlimited:

```json
{
  "maxEntriesPerIP": 3
}
```

The repository enforces the limit with an IP count map under the queue lock,
using expected O(1) work per admission or departure. HTTP `POST /enter` returns
429 with a JSON message when the limit is reached, without adding a ticket.
The client IP comes from the connection address unless `clientIPHeader` is set.
For example, `"clientIPHeader": "X-Real-IP"` uses only that header, with
case-insensitive header name matching. Configure your reverse proxy to overwrite
this header with the client IP and restrict direct access to the service when
using it. Comma-separated address lists (including X-Forwarded-For chains),
multiple header values, empty values, and addresses with ports are rejected.
Rejection logs include the method, path, connection address, and reason.
Equivalent IPv4 and IPv4-mapped IPv6 addresses share a count.

Finishing or expiring a ticket releases its slot. Clearing the active queue
resets all counts. Finished tickets do not count toward the limit.
Persistence recovery restores every active ticket and rebuilds counts even if
an IP exceeds the configured limit. New entries for that IP are rejected until
its active count falls below the limit. The persistence format is unchanged.

## HTTPS

Set `tlsCertFile` and `tlsKeyFile` to the PEM certificate chain and private key
file paths in `gohvq_config.json`, for example:

```json
{
  "serverPort": 4242,
  "tlsCertFile": "certs/fullchain.pem",
  "tlsKeyFile": "certs/privkey.pem"
}
```

Relative paths resolve from the working directory. With both paths set, the
configured port serves HTTPS exclusively. Plain HTTP requests are rejected
without reaching API handlers; there is no HTTP listener or redirect service.
When both values are omitted or empty, the server uses HTTP as before.
Providing only one nonempty path, unreadable files, or an invalid certificate/key
pair causes startup to fail without falling back to HTTP. Certificates are loaded
at startup; restart the service after replacing them.
