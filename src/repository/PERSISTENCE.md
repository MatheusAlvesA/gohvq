# Client IPs and persistence

`POST /enter` captures the connection address from `RemoteAddr`, without the
port or IPv6 zone. IPv4-mapped IPv6 addresses are normalized to IPv4. The IP
belongs to the original entry and is not changed by `GET /position`.
Forwarding headers are ignored; behind a reverse proxy, the recorded address
is the proxy's address. Supporting original addresses behind proxies requires
an explicit trusted-proxy configuration.

Only authenticated `POST /admin/finishItems` and
`GET /admin/finishedItem/{key}` responses expose the `ip` field. Public entry
and position responses omit it, including position responses for finished
items. Repository callers pass an IP to `CreateItem`; an empty string represents
an unknown address.

Each persistence record contains:

- One status byte (`A`, `F`, or `X`) and one ASCII space.
- Exactly `KEY_SIZE` key bytes and one ASCII space.
- An IP field of exactly `PERSISTENCE_IP_SIZE` (45) bytes, right-padded with ASCII spaces.
- One LF newline.

The record size is `KEY_SIZE + 4 + PERSISTENCE_IP_SIZE` bytes (69 with the
current key size). The IP field accommodates IPv4, IPv6, and mixed IPv6/IPv4
text. An unknown IP is represented by an all-space field. Finish and delete
update only the status byte. Compaction and recovery retain the IP.

