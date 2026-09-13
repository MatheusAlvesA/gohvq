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
| `recaptchaSecretKey` | String | `""` | Google reCAPTCHA v2 secret key; enables CAPTCHA on `POST /enter`. Configure only one provider. |
| `turnstileSecretKey` | String | `""` | Cloudflare Turnstile secret key; enables CAPTCHA on `POST /enter`. Configure only one provider. |
| `persistenceEnabled` | Boolean | `true` | Recover and asynchronously persist queue data in `gohvq_persistence.db` in the working directory. When `false`, queue data exists only in memory for that run. |
| `clearMaxSeconds` | Nonnegative integer | `1` | Time budget in seconds for each cleanup scan. `0` gives an immediate timeout budget; it does not disable the budget. |
| `pingTimeout` | Nonnegative integer | `60` | An active ticket becomes eligible for removal when the seconds since its last ping exceed this value. `GET /position?key=...` refreshes the ping of an active ticket. `0` does not disable expiration. |
| `clearFrequency` | Nonnegative integer | `10` | Minimum interval in seconds between the end of a cleanup scan and the next scan. The worker checks whether cleanup is due every 500 milliseconds. `0` makes cleanup eligible on every worker tick. |
| `maxQueueSize` | Nonnegative integer | `0` | Maximum total active tickets. `0` means unlimited. Uses the item count, independent of stale positions. |
| `maxEntriesPerIP` | Nonnegative integer | `0` | Maximum active tickets per IP. `0` means unlimited. See the admission behavior below. |

For example, this configures the service and allows three active tickets
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

## CAPTCHA on queue entry

Set exactly one private key in `gohvq_config.json` and restart:

```json
{
  "recaptchaSecretKey": "your-google-recaptcha-v2-secret"
}
```

Or use Cloudflare:

```json
{
  "turnstileSecretKey": "your-cloudflare-turnstile-secret"
}
```

When both keys are omitted or empty, CAPTCHA is disabled and `POST /enter`
continues to accept an empty body. If both keys are nonempty, entry returns
503 until the configuration is corrected and the service restarted. Other
routes remain available and do not require CAPTCHA.

The frontend must render the chosen provider's widget using its public site key
and send the resulting token to this API. Keep the private key on the server.
Google support targets reCAPTCHA v2 (including Invisible v2), not v3 or
Enterprise; score-based responses are rejected. Configure allowed domains in
the provider's dashboard.

For either provider, send JSON:

```sh
curl -X POST http://localhost:4242/enter \
  -H 'Content-Type: application/json' \
  --data '{"captchaToken":"token-from-the-widget"}'
```

Alternatively, send `application/x-www-form-urlencoded` with exactly one
`g-recaptcha-response` field for Google or `cf-turnstile-response` for Cloudflare.
Tokens in query parameters are not accepted. Request bodies are limited to
16 KiB, Google tokens to 8192 bytes, and Turnstile tokens to 2048 bytes.

Validation uses the provider's HTTPS Siteverify endpoint before creating a
ticket, without holding queue locks. Each verification has a five-second
timeout, does not follow redirects, and is cancelled when the request is
cancelled. No extra dependency or repository scan is introduced.

- 400: missing/invalid token, malformed or oversized body, or unsupported content type.
- 403: the provider rejects the token, including expired or reused tokens.
- 503: conflicting provider configuration, network/timeout failure, non-200
  provider response, or malformed/oversized provider response.
- 201: verification succeeded and the ticket was created; existing admission
  errors such as the per-IP limit (429) still apply.

Rejected entries do not change queue state. Tokens are single-use; obtain a
fresh token before retrying, including after an admission error. Without keys,
the server does not parse a CAPTCHA body or contact either provider.

Protocol references: [Google verification](https://developers.google.com/recaptcha/docs/verify)
and [Cloudflare validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/).

## Total queue capacity

Set `"maxQueueSize": 1000` to allow at most 1,000 active tickets across all IPs.
Omitting the setting or using zero leaves the queue unlimited. Admission checks
the active item count under the same lock as insertion, in O(1) time.
The last ticket's position is not used.

When full, `POST /enter` returns HTTP 503 with
`{"message":"Queue is full. Please try again later."}` without adding a ticket.
Finishing, removing expired tickets during cleanup, or clearing the queue frees
capacity; finished tickets do not count. The per-IP limit still applies.

Persistence recovery preserves all saved tickets even when their count exceeds
the configured capacity. New admissions wait until the active count falls below
the limit. Restart the service to apply configuration changes.
